# `--prefer-hardlinks`

The `--prefer-hardlinks` flag tells Lilt to create filesystem hardlinks instead of performing byte-by-byte copies for any file that does not require audio transcoding.

## What it does

When `--prefer-hardlinks` is enabled, Lilt attempts to hardlink the following files instead of copying them:

- 16-bit FLAC files at 44.1kHz or 48kHz
- 16-bit ALAC files at 44.1kHz or 48kHz
- MP3 files
- JPG and PNG image files (when `--copy-images` is also enabled)
- Fallback copies after conversion or metadata-read failures

## Why use it

If your source and target directories are on the same filesystem, hardlinks let you keep two directory trees without duplicating the underlying file data. This can save a large amount of disk space for big music libraries.

## Usage

```bash
lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --prefer-hardlinks
```

You can combine it with other flags:

```bash
lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --prefer-hardlinks --copy-images
```

## Fallback behavior

If a hardlink cannot be created for any reason, Lilt automatically falls back to the normal copy behavior and prints a warning such as:

```
Warning: Could not create hardlink for /path/to/source.flac, falling back to copy: invalid cross-device link
```

Common reasons for hardlink failure include:

- Source and target are on different filesystems or devices
- The target filesystem does not support hardlinks (e.g., FAT32, exFAT)
- Permission issues
- The destination file exists and cannot be removed

## Important caveats

### Shared inode data

Because hardlinks share the same inode, modifying the source file after conversion also modifies the target file, and vice versa. If you need independent copies, do not use this flag.

### Overwrite semantics

If the destination file already exists, Lilt removes it before attempting the hardlink, matching the existing copy behavior.

### Docker mode

`copyFile()` runs on the host after containerized conversion finishes. Whether hardlinks work depends on the host filesystem layout, not the container mounts. If `/source` and `/target` point to the same host filesystem, hardlinks succeed; otherwise, they fall back to copy.

## Requirements

- Source and target paths must reside on the same filesystem.
- The filesystem must support hardlinks (NTFS, APFS, ext4, etc.).

## Examples

### Save space when re-encoding a library on the same drive

```bash
lilt /mnt/music/library --target-dir /mnt/music/library-16bit --prefer-hardlinks
```

### Copy album artwork as hardlinks too

```bash
lilt ~/Music/Album --target-dir ~/Music/Album-16bit --prefer-hardlinks --copy-images
```

## See also

- [README.md](../README.md) for general usage
- `--enforce-output-format` for converting all files to a specific format
- `--copy-images` for copying album artwork
