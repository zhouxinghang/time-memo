`quadrants.heic` is a synthetic 2400 × 1200 test image created for this project.
Its quadrants are red, green, blue, and white (reading left to right, top to
bottom). It contains a grid of HEVC tiles and an `irot` property set to zero.

The source PNG was drawn with Go's `image` package and converted with macOS:

```sh
sips -s format heic quadrants.png --out quadrants.heic
```

Tests change the major brand to `mif1` for HEIF coverage and the `irot` value
to one for a 90° counterclockwise rotation. No personal photos are included.
