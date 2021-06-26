[photos: RAW Photo library manager](https://github.com/frizinak/photos)
===

## Dependencies

- imagemagick         (`-action previews` if rawtherapee not in PATH)
- libgphoto2          (`-action import`) if not compiled with `-tags gphoto2cli` (preferred)
- gphoto2             (`-action import`) if compiled with `-tags gphoto2cli`
- ffmpeg for .MOVs    (`-action import`, currently only uses ffprobe for metadata)
- rawtherapee         (`-action convert`)

## What it does

1. Imports images to `-raws`
2. Symlinks those raws to `-collection` in an opinionated directory hierarchy
3. At this point you can:
    - rename and/or move any of those symlink (as long as the symlink stays intact and remains a descendant of `-collection`)
    - review em using the builtin rating application `-action rate`
    - edit the raws in `-collection` with rawtherapee
4. Ratings and deleted/trash flag will be synced between all symlinks of a given raw
5. Convert images to jpegs using the rawtherapee.pp3 sidecar file and store em in `-jpegs` using the same directory hierarchy as the one we/you have created in `-collection`

## Usage

`photos -h`

also see the examples in repo

### required flags:

`photos -raws <rawDir> -collection <collectionDir> -jpegs <jpegDir>`

or 

`photos -base <basedir>`

which defaults to

`photos -raws <basedir>/Originals -collection <basedir>/Collection -jpegs <basedir>/Converted`

### common usage

- Import photos if camera is connected.
- Create symlinks to my_library/Collection.
- Sync metadata between rawtherapees .pp3 and our own .meta files.
- Check symlinks again in case a metadata file indicated a delete.
- Generate previews.

`photos -base my_library -action import,link,sync-meta,link,previews

- Rate images.
- Sync metadata to rawtherapees .pp3 files.

`photos -base my_library -action rate,sync-meta,link -filter unrated`

- Remove converted images and pp3s whose RAWs have been deleted and/or those with a low rating.

`photos -base my_library -action cleanup -gt 2`

- Convert images with a rating > 2 and have been opened in rawtherapee (-edited) to jpegs

`photos -base my_library -action convert -sizes 3840,1920,800 -filter undeleted,edited -gt 2

- Merge library `two` onto `one`

`rsync -ua two/ one`

## Install

`go install github.com/frizinak/photos/cmd/photos@latest`

## Bash completion

`go install github.com/frizinak/photos/cmd/photos_completion@latest`

`complete -C photos_completion -o default photos`
