// timememo-user is the only supported user provisioning interface.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
	"timememo/internal/store"
)

func run() error {
	if len(os.Args) < 2 || os.Args[1] != "create" {
		return fmt.Errorf("用法：timememo-user create --username alice --name 昵称 [--password-stdin]")
	}
	f := flag.NewFlagSet("create", flag.ContinueOnError)
	username := f.String("username", "", "登录用户名")
	name := f.String("name", "", "显示昵称")
	stdin := f.Bool("password-stdin", false, "从标准输入读取密码（自动化使用）")
	if err := f.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *username == "" || *name == "" {
		return fmt.Errorf("--username 和 --name 必填")
	}
	var password string
	if *stdin {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("未读取到密码")
		}
		password = strings.TrimSuffix(scanner.Text(), "\r")
	} else {
		fmt.Fprint(os.Stderr, "密码（至少 12 字节，输入不会显示）：")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("无法读取密码，请在终端运行，或使用 --password-stdin")
		}
		fmt.Fprint(os.Stderr, "再次输入密码：")
		confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		if string(b) != string(confirm) {
			return fmt.Errorf("两次密码不一致")
		}
		password = string(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer s.DB.Close()
	if err = s.Migrate(ctx); err != nil {
		return err
	}
	u, err := s.CreateUser(ctx, *username, *name, password)
	if err != nil {
		return fmt.Errorf("创建失败（请检查用户名是否重复）：%w", err)
	}
	fmt.Printf("已创建用户 %s（%s，ID %d）\n", u.DisplayName, u.Username, u.ID)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
