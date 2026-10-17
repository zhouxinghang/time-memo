#!/usr/bin/env python3
"""Convert published WordPress WXR posts/comments to TimeMemo PostgreSQL SQL.

Only writes files. Users must already exist (provision them with timememo-user).
Uses the Python standard library; does not connect to WordPress or a database.
"""

import argparse
from collections import Counter
from dataclasses import dataclass, field
from datetime import datetime, timezone
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
from urllib.parse import quote, urljoin, urlsplit
import xml.etree.ElementTree as ET
from zoneinfo import ZoneInfo


NS = {
    "wp": "http://wordpress.org/export/1.2/",
    "dc": "http://purl.org/dc/elements/1.1/",
    "content": "http://purl.org/rss/1.0/modules/content/",
}
VOID_TAGS = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}


def text(element, name):
    return element.findtext(name, default="", namespaces=NS)


def literal(value):
    if "\x00" in value:
        raise ValueError("PostgreSQL text cannot contain NUL bytes")
    # The SQL sets standard_conforming_strings=on before reading these values.
    return "'" + value.replace("'", "''") + "'"


def array(values):
    return "ARRAY[" + ", ".join(literal(v) for v in values) + "]::text[]"


def escape_markdown(value):
    value = re.sub(r"([\\`*_\[\]<>|])", r"\\\1", value)
    value = re.sub(r"(?m)^(\s*)(#{1,6})(?=\s)", r"\1\\\2", value)
    return re.sub(r"(?<!\n)\n(?!\n)", "  \n", value)


@dataclass
class Node:
    tag: str
    attrs: dict = field(default_factory=dict)
    children: list = field(default_factory=list)


class Markdown(HTMLParser):
    def __init__(self, base_url, media):
        super().__init__(convert_charrefs=True)
        self.root = Node("root")
        self.stack = [self.root]
        self.base_url = base_url
        self.media = media

    def handle_starttag(self, tag, attrs):
        node = Node(tag, dict(attrs))
        self.stack[-1].children.append(node)
        if tag not in VOID_TAGS:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in VOID_TAGS:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        for index in range(len(self.stack) - 1, 0, -1):
            if self.stack[index].tag == tag:
                del self.stack[index:]
                break

    def handle_data(self, data):
        self.stack[-1].children.append(data)

    def url(self, value):
        absolute = urljoin(self.base_url.rstrip("/") + "/", value)
        if urlsplit(absolute).scheme.lower() not in {"http", "https", "mailto"}:
            return ""
        return quote(absolute, safe="/:#?&=@%+~!$,;*[]")

    def render(self, node):
        if isinstance(node, str):
            return escape_markdown(node)
        tag = node.tag
        if tag in {"script", "style"}:
            return ""
        if tag == "img":
            src = node.attrs.get("src", "")
            alt = node.attrs.get("alt", "").strip() or "原站图片"
            url = self.url(src) if src else ""
            self.media.append({"original_src": src, "url": url, "alt": alt})
            return f"[{escape_markdown(alt)}]({url})" if url else "[原站图片地址为空或不受支持]"
        if tag == "br":
            return "  \n"
        if tag == "hr":
            return "\n\n---\n\n"
        if tag in {"pre", "code"}:
            def raw(n):
                return n if isinstance(n, str) else "".join(raw(c) for c in n.children)
            value = raw(node)
            fence = "`" * max(3 if tag == "pre" else 1, max((len(m) + 1 for m in re.findall(r"`+", value)), default=1))
            return f"\n\n{fence}\n{value.rstrip()}\n{fence}\n\n" if tag == "pre" else f"{fence} {value} {fence}"
        if tag in {"ul", "ol"}:
            lines = []
            for child in node.children:
                if isinstance(child, Node) and child.tag == "li":
                    body = self.render(child).strip()
                    marker = f"{len(lines) + 1}. " if tag == "ol" else "- "
                    lines.append(marker + body.replace("\n", "\n" + " " * len(marker)))
            return "\n\n" + "\n".join(lines) + "\n\n"
        value = "".join(self.render(child) for child in node.children)
        if tag in {"p", "div", "section", "article", "figure", "figcaption"}:
            return "\n\n" + value.strip() + "\n\n" if value.strip() else ""
        if re.fullmatch(r"h[1-6]", tag):
            return "\n\n" + "#" * int(tag[1]) + " " + value.strip() + "\n\n"
        if tag == "blockquote":
            return "\n\n" + "\n".join("> " + line for line in value.strip().splitlines()) + "\n\n"
        if tag in {"strong", "b"}:
            return "**" + value.strip() + "**"
        if tag in {"em", "i"}:
            return "*" + value.strip() + "*"
        if tag in {"del", "s"}:
            return "~~" + value.strip() + "~~"
        if tag == "a":
            url = self.url(node.attrs.get("href", "")) if node.attrs.get("href") else ""
            if any(isinstance(child, Node) and child.tag == "img" for child in node.children):
                return value + (f"（[原图]({url})）" if url else "")
            return f"[{value.strip() or url}]({url})" if url else value
        return value

    def convert(self, source):
        self.feed(source.replace("\r\n", "\n").replace("\r", "\n"))
        self.close()
        result = self.render(self.root)
        return re.sub(r"\n(?:[ \t]*\n){2,}", "\n\n", result).strip()


def timestamp(element, kind, warnings):
    gmt = text(element, f"wp:{kind}_date_gmt")
    local = text(element, f"wp:{kind}_date")
    if gmt and not gmt.startswith("0000-"):
        return datetime.strptime(gmt, "%Y-%m-%d %H:%M:%S").replace(tzinfo=timezone.utc).isoformat()
    if local and not local.startswith("0000-"):
        warnings.append(f"{kind} #{text(element, 'wp:' + kind + '_id')} 无 GMT 时间，按 Asia/Shanghai 解释本地时间。")
        return datetime.strptime(local, "%Y-%m-%d %H:%M:%S").replace(tzinfo=ZoneInfo("Asia/Shanghai")).astimezone(timezone.utc).isoformat()
    raise ValueError(f"Missing valid {kind} timestamp")


def check_length(value, limit, label):
    if not 1 <= len(value) <= limit:
        raise ValueError(f"{label}: length {len(value)} is outside 1–{limit}; refusing to truncate")


def sql_file(authors, posts, comments, source_name, digest):
    author_values = ",\n".join(f"    ({a['wp_id']}, {literal(a['login'])}, {literal(a['username'])})" for a in authors)
    post_values = ",\n".join(
        f"    ({p['wp_id']}, {p['author']}, {literal(p['title'])}, {literal(p['body'])}, {literal(p['category'])}, {array(p['tags'])}, {literal(p['created_at'])})"
        for p in posts
    )
    comment_values = ",\n".join(
        f"    ({c['wp_id']}, {c['post']}, {c['author']}, {literal(c['body'])}, {literal(c['created_at'])})"
        for c in comments
    )
    return f"""-- WordPress → TimeMemo / PostgreSQL 16+
-- Source: {source_name.replace(chr(10), ' ').replace(chr(13), ' ')}
-- SHA-256: {digest}
-- Published posts: {len(posts)}; approved comments: {len(comments)}.
-- Requires the project's schema and existing users. Edit target_username below if needed.
-- Does not create users, overwrite content, disable triggers, or fetch external images.
-- Exact existing records are reused; duplicate source comments retain their multiplicity.
-- Run with: psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -f import.sql

BEGIN;
SET LOCAL standard_conforming_strings = on;
SET LOCAL client_encoding = 'UTF8';
SET LOCAL timezone = 'UTC';

CREATE TEMP TABLE _timememo_wp_authors (
    wp_user_id bigint PRIMARY KEY,
    source_login text NOT NULL,
    target_username text NOT NULL,
    target_user_id bigint
) ON COMMIT DROP;

-- Account mapping only: users are provisioned separately with timememo-user.
INSERT INTO _timememo_wp_authors (wp_user_id, source_login, target_username) VALUES
{author_values};

CREATE TEMP TABLE _timememo_wp_posts (
    wp_post_id bigint PRIMARY KEY,
    wp_user_id bigint NOT NULL REFERENCES _timememo_wp_authors(wp_user_id),
    title text NOT NULL,
    body text NOT NULL,
    category text NOT NULL,
    tags text[] NOT NULL,
    created_at timestamptz NOT NULL,
    target_post_id bigint
) ON COMMIT DROP;

INSERT INTO _timememo_wp_posts (wp_post_id, wp_user_id, title, body, category, tags, created_at) VALUES
{post_values};

CREATE TEMP TABLE _timememo_wp_comments (
    wp_comment_id bigint PRIMARY KEY,
    wp_post_id bigint NOT NULL REFERENCES _timememo_wp_posts(wp_post_id),
    wp_user_id bigint NOT NULL REFERENCES _timememo_wp_authors(wp_user_id),
    body text NOT NULL,
    created_at timestamptz NOT NULL
) ON COMMIT DROP;
""" + (f"""
INSERT INTO _timememo_wp_comments (wp_comment_id, wp_post_id, wp_user_id, body, created_at) VALUES
{comment_values};
""" if comments else "") + f"""
-- Serialize content/user writes for the short import transaction, including HTTP writes.
LOCK TABLE users, posts, comments IN SHARE ROW EXCLUSIVE MODE;

DO $timememo_wordpress_import$
DECLARE
    missing_usernames text;
    added_posts integer;
    added_comments integer;
BEGIN
    SELECT string_agg(a.target_username, ', ' ORDER BY a.wp_user_id)
      INTO missing_usernames
      FROM _timememo_wp_authors a
      LEFT JOIN users u ON u.username = a.target_username
     WHERE u.id IS NULL;
    IF missing_usernames IS NOT NULL THEN
        RAISE EXCEPTION 'Missing TimeMemo users: %. Create with timememo-user or edit target_username mapping.', missing_usernames;
    END IF;

    UPDATE _timememo_wp_authors a SET target_user_id = u.id
      FROM users u WHERE u.username = a.target_username;

    -- This export has no duplicate post payloads. Refuse ambiguous account remapping.
    IF EXISTS (
        SELECT 1 FROM _timememo_wp_posts s
        JOIN _timememo_wp_authors a USING (wp_user_id)
        GROUP BY a.target_user_id, s.title, s.body, s.category, s.tags, s.created_at
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'Author mapping creates identical posts; keep separate author accounts.';
    END IF;

    INSERT INTO posts (author_id, title, body, category, tags, created_at)
    SELECT a.target_user_id, s.title, s.body, s.category, s.tags, s.created_at
      FROM _timememo_wp_posts s
      JOIN _timememo_wp_authors a USING (wp_user_id)
     WHERE NOT EXISTS (
        SELECT 1 FROM posts p
         WHERE p.author_id = a.target_user_id AND p.title = s.title AND p.body = s.body
           AND p.category = s.category AND p.tags = s.tags AND p.created_at = s.created_at
     )
     ORDER BY s.wp_post_id;
    GET DIAGNOSTICS added_posts = ROW_COUNT;

    -- Target identity IDs are allocated normally; never reuse WordPress numeric IDs.
    UPDATE _timememo_wp_posts s
       SET target_post_id = (
           SELECT p.id FROM posts p
           JOIN _timememo_wp_authors a ON a.wp_user_id = s.wp_user_id
            WHERE p.author_id = a.target_user_id AND p.title = s.title AND p.body = s.body
              AND p.category = s.category AND p.tags = s.tags AND p.created_at = s.created_at
            ORDER BY p.id LIMIT 1
       );
    IF EXISTS (SELECT 1 FROM _timememo_wp_posts WHERE target_post_id IS NULL) THEN
        RAISE EXCEPTION 'Could not resolve imported post IDs';
    END IF;

    -- Compare occurrence counts, so 3 identical source comments remain 3 after reruns.
    WITH ranked AS (
        SELECT c.*, p.target_post_id, a.target_user_id,
               row_number() OVER (
                   PARTITION BY p.target_post_id, a.target_user_id, c.body, c.created_at
                   ORDER BY c.wp_comment_id
               ) AS occurrence
          FROM _timememo_wp_comments c
          JOIN _timememo_wp_posts p USING (wp_post_id)
          JOIN _timememo_wp_authors a ON a.wp_user_id = c.wp_user_id
    )
    INSERT INTO comments (post_id, author_id, body, created_at)
    SELECT r.target_post_id, r.target_user_id, r.body, r.created_at
      FROM ranked r
     WHERE r.occurrence > (
        SELECT count(*) FROM comments c
         WHERE c.post_id = r.target_post_id AND c.author_id = r.target_user_id
           AND c.body = r.body AND c.created_at = r.created_at
     )
     ORDER BY r.wp_comment_id;
    GET DIAGNOSTICS added_comments = ROW_COUNT;

    RAISE NOTICE 'WordPress import: % posts inserted, % reused; % comments inserted, % reused.',
        added_posts, {len(posts)} - added_posts, added_comments, {len(comments)} - added_comments;
END;
$timememo_wordpress_import$;

COMMIT;
"""


def convert(source, output):
    raw = source.read_bytes()
    if re.search(br"<!\s*(?:DOCTYPE|ENTITY)\b", raw, re.I):
        raise ValueError("DTD/entity declarations are not supported")
    channel = ET.fromstring(raw).find("channel")
    if channel is None or channel.find("wp:wxr_version", NS) is None:
        raise ValueError("Expected a WordPress WXR 1.2 export")
    if text(channel, "wp:wxr_version") != "1.2":
        raise ValueError("Only WordPress WXR 1.2 is supported")
    base_url = text(channel, "wp:base_blog_url") or text(channel, "link")
    if urlsplit(base_url).scheme not in {"http", "https"} or not urlsplit(base_url).netloc:
        raise ValueError("The export must supply an absolute HTTP(S) site URL")

    authors = {}
    logins = {}
    warnings, posts, comments, media, skipped = [], [], [], [], []
    for author in channel.findall("wp:author", NS):
        wp_id = int(text(author, "wp:author_id"))
        login = text(author, "wp:author_login")
        if wp_id in authors or login in logins:
            raise ValueError("Duplicate WordPress author ID/login")
        username = login.lower()
        if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{2,31}", username):
            username = f"wp_user_{wp_id}"
        authors[wp_id] = {"wp_id": wp_id, "login": login, "username": username, "display_name": text(author, "wp:author_display_name") or login}
        logins[login] = wp_id

    for item in channel.findall("item"):
        wp_id = int(text(item, "wp:post_id"))
        kind, status = text(item, "wp:post_type"), text(item, "wp:status")
        if kind != "post" or status != "publish":
            skipped.append({"wp_id": wp_id, "type": kind, "status": status, "title": text(item, "title"), "comments": len(item.findall("wp:comment", NS))})
            continue
        login = text(item, "dc:creator")
        if login not in logins:
            raise ValueError(f"Unknown creator for post #{wp_id}: {login}")
        title = text(item, "title").strip()
        check_length(title, 120, f"post #{wp_id} title")
        post_media = []
        body = Markdown(base_url, post_media).convert(text(item, "content:encoded"))
        check_length(body, 50000, f"post #{wp_id} body")
        source_categories = [c.text or "" for c in item.findall("category") if c.get("domain") == "category"]
        source_tags = [c.text or "" for c in item.findall("category") if c.get("domain") == "post_tag"]
        category = "archive"
        tags = list(dict.fromkeys(source_tags + [c for c in source_categories if c != "未分类"]))
        if len(tags) > 5:
            raise ValueError(f"post #{wp_id} has more than 5 tags; refusing to discard tags")
        for tag in tags:
            check_length(tag, 20, f"post #{wp_id} tag")
        posts.append({"wp_id": wp_id, "author": logins[login], "title": title, "body": body, "category": category, "tags": tags, "created_at": timestamp(item, "post", warnings)})
        media.extend({"wp_post_id": wp_id, "title": title, **m} for m in post_media)
        comment_index = {int(text(c, "wp:comment_id")): c for c in item.findall("wp:comment", NS)}
        for comment in item.findall("wp:comment", NS):
            comment_id = int(text(comment, "wp:comment_id"))
            if text(comment, "wp:comment_approved") != "1" or text(comment, "wp:comment_type") not in {"", "comment"}:
                skipped.append({"wp_id": comment_id, "type": "comment", "status": text(comment, "wp:comment_approved"), "title": f"post #{wp_id}"})
                continue
            user_id = int(text(comment, "wp:comment_user_id") or "0")
            original_author = text(comment, "wp:comment_author")
            comment_media = []
            comment_body = Markdown(base_url, comment_media).convert(text(comment, "wp:comment_content"))
            if user_id == 0:
                authors.setdefault(0, {"wp_id": 0, "login": "(guest)", "username": "wp_guest", "display_name": "WordPress 访客"})
                comment_body = f"**WordPress 访客：{escape_markdown(original_author)}**\n\n" + comment_body
            if user_id not in authors:
                raise ValueError(f"Unknown user #{user_id} in comment #{comment_id}")
            parent_id = int(text(comment, "wp:comment_parent") or "0")
            if parent_id:
                parent = comment_index.get(parent_id)
                parent_author = text(parent, "wp:comment_author") if parent is not None else "未知作者"
                comment_body = f"> 回复 WordPress 评论 #{parent_id}（{escape_markdown(parent_author)}）\n\n" + comment_body
            check_length(comment_body, 5000, f"comment #{comment_id} body")
            comments.append({"wp_id": comment_id, "post": wp_id, "author": user_id, "body": comment_body, "created_at": timestamp(comment, "comment", warnings), "parent": parent_id, "source_author": original_author})
            media.extend({"wp_post_id": wp_id, "wp_comment_id": comment_id, "title": title, **m} for m in comment_media)

    if not posts:
        raise ValueError("No published posts to import")
    used_authors = {p["author"] for p in posts} | {c["author"] for c in comments}
    authors = [authors[k] for k in sorted(used_authors)]
    if len({a["username"] for a in authors}) != len(authors):
        raise ValueError("Generated target usernames collide; resolve author names first")
    keys = [(p["author"], p["title"], p["body"], p["category"], tuple(p["tags"]), p["created_at"]) for p in posts]
    if len(set(keys)) != len(keys):
        raise ValueError("Export contains identical post payloads; source ID mapping is required")
    digest = hashlib.sha256(raw).hexdigest()
    sql = sql_file(authors, posts, comments, source.name, digest)
    duplicate_comments = {str(ids): count for ids, count in Counter((c["post"], c["author"], c["body"], c["created_at"]) for c in comments).items() if count > 1}
    report = {
        "source_file": source.name, "source_sha256": digest, "site_url": base_url,
        "scope": "published posts and their approved regular comments",
        "posts": len(posts), "comments": len(comments), "authors": authors,
        "categories": dict(Counter(p["category"] for p in posts)),
        "post_date_range_utc": [min(p["created_at"] for p in posts), max(p["created_at"] for p in posts)],
        "guest_comments": sum(c["author"] == 0 for c in comments),
        "replies": [{"wp_comment_id": c["wp_id"], "wp_parent_id": c["parent"]} for c in comments if c["parent"]],
        "identical_comment_groups": len(duplicate_comments),
        "media_references": len(media), "skipped": skipped, "warnings": warnings,
    }
    skipped_rows = "\n".join(f"| {s['wp_id']} | {s['type']} | {s['status']} | {s['title'].replace('|', '／')} |" for s in skipped)
    author_rows = "\n".join(f"| {a['wp_id']} | {a['login']} | `{a['username']}` | {a['display_name']} |" for a in authors)
    instructions = f"""# WordPress 导入 TimeMemo

源文件：`{source.name}`，SHA-256：`{digest}`。

已生成 **{len(posts)} 篇已发布文章、{len(comments)} 条已审核普通评论**，不执行导入。正文已从 WordPress HTML / Gutenberg 转为项目支持的 Markdown；中文、段落、换行、标题、列表、链接和原始发布时间保留。以 `*_date_gmt` 为准，SQL 显式使用 UTC 时间（不会受数据库服务器时区影响）。

## 先准备账号

项目只通过 `timememo-user` CLI 创建账号。XML 不含密码；SQL 不创建账号或更改密码。下面是 SQL 开头的默认映射，已有账号可直接复用；如目标用户名不同，先编辑 `import.sql` 中 `_timememo_wp_authors` 的 `target_username` 值。

| WordPress 用户 ID | 原登录名 | TimeMemo 用户名 | 原显示名 |
| --- | --- | --- | --- |
{author_rows}

缺少的账号通过现有 CLI 交互创建，例如：

```sh
./bin/timememo-user create --username xinghang --name xinghang
```

Docker 部署使用 `docker compose exec app /app/timememo-user create --username xinghang --name xinghang`。其他用户名按映射表替换。`wp_guest` 是 {report['guest_comments']} 条匿名访客评论共用的归档账号（昵称可用“WordPress 访客”）；原访客名保留在评论正文。为该账号设置随机密码即可，不需要向访客提供登录。

## 执行

先由应用启动或用户 CLI 完成建表，在项目根目录执行（将路径改为本目录）：

```sh
psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -f '{output.relative_to(Path.cwd()) if output.is_relative_to(Path.cwd()) else output}/import.sql'
```

Compose 数据库也可执行：

```sh
docker compose exec -T postgres psql -U timememo -d timememo -X -v ON_ERROR_STOP=1 < '{output.relative_to(Path.cwd()) if output.is_relative_to(Path.cwd()) else output}/import.sql'
```

`DATABASE_URL` 须指向目标库；Go 程序和 shell 不会自动加载 `.env`。执行期间 SQL 短暂锁定账号、文章与评论写入，在一个事务中完成；账号缺失或约束不满足会整体回滚。数据库必须是 UTF-8。

文章 ID 和评论 ID 由目标数据库正常分配，因此可导入已有内容的数据库，不会覆盖相同数字 ID 的旧记录。不使用 `UPDATE` / `DELETE` / `TRUNCATE` 修改正式内容表，也不禁用不可变触发器。临时表只在本事务内存在。

同一份 SQL、同一份账号映射可重复执行：完全相同的文章复用，评论按相同文章、作者、正文和时间的出现次数补足，原导出中完全相同但 ID 不同的评论仍保留各自条数。这不是基于 WordPress ID 的增量同步；改变账号映射或内容后再执行会视为另一份数据。

## 转换约定

- 所有导入文章统一归入 `archive`。非“未分类”的原分类名作为标签保留，原文章标签也保留。导入后分类统计：`{json.dumps(report['categories'], ensure_ascii=False)}`。
- 当前项目评论只有平铺结构。{len(report['replies'])} 条回复在正文开头标注原父评论 ID 与作者；关系清单保留在 `report.json`。
- 未迁移 WordPress 密码、角色、邮箱、IP、头像、菜单、站点设置、SEO、修订、置顶、原网址和修改时间等项目没有对应字段的数据。原 WordPress 用户 ID、文章 ID、评论 ID 保留在 SQL 临时数据中，数据库使用新 ID。
- {len(media)} 个原站图片引用已转换为普通图片链接，详细地址见 `media-manifest.json`。XML 不含图片二进制，也没有附件条目。项目只渲染 `/media/<32位哈希>.jpg` 的本地图片，因此 SQL 没有伪造 `uploads` / `post_images` 记录。链接未联网验证，原站地址 `{base_url}` 可能需要在可访问的环境或按实际原站域名使用。若要站内显示图片，需要原图文件并经过项目的图片处理流程；文章发布后不可编辑，建议正式导入前确定是否补齐图片。
- 已审核的 WordPress 默认示例评论和含广告外链评论均按原数据保留，没有额外筛除。

## 排除记录

按确认范围，排除草稿、私密文章、页面和导航项，共 {len(skipped)} 条：

| WordPress ID | 类型 | 状态 | 标题 |
| --- | --- | --- | --- |
{skipped_rows}

导出文件与生成数据包含私人内容，输出放在已被 `.gitignore` 忽略的 `data/` 目录，避免随项目源码提交。
"""
    output.mkdir(parents=True, exist_ok=True)
    (output / "import.sql").write_text(sql, encoding="utf-8")
    (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (output / "media-manifest.json").write_text(json.dumps(media, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (output / "README.md").write_text(instructions, encoding="utf-8")
    print(f"Generated {len(posts)} posts, {len(comments)} comments, {len(media)} image links → {output / 'import.sql'}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path, help="WordPress WXR 1.2 XML file")
    parser.add_argument("--output-dir", type=Path, required=True, help="Private output directory, preferably under data/imports/")
    args = parser.parse_args()
    convert(args.source.resolve(), args.output_dir.resolve())
