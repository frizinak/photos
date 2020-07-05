# RAW Photo library manager

## Dependencies

imagemagick
gphoto2
ffmpeg for .MOVs
rawtherapee to use `-actions convert`

## What it does

1. Imports images to `-raws`
2. Symlinks those raws to `-collection` in an opinionated directory hierarchy
3. At this point you can:
    - rename and/or move any of those symlink (as long as the symlink stays intact and remains a descendant of `-raws`)
    - review em using the builtin rating application `-actions rate`
    - edit these raw symlinks in rawtherapee
4. Ratings and deleted/trash flag will be synced between all symlinks of a given raw
5. Convert images to jpegs using the rawtherapee.pp3 sidecar file and store em in `-jpegs` using the same directory hierarchy as the one we/you have created in `-collection`

## Usage

`photos -h`

### required flags:

`photos -raws <rawDir> -collection <collectionDir> -jpegs <jpegDir>`

- or -

`photos -base <basedir>`

### common usage

- Import photos if camera is connected.
- Create symlinks to my_library/Collection.
- Sync metadata between rawtherapees .pp3 and our own .meta files.
- Check symlinks again in case a metadata file indicated a delete.
- Generate previews.

`photos -base my_library -actions import,link,sync-meta,link,previews -filter all`

- Rate images.
- Sync metadata to rawtherapees .pp3 files.

`photos -base my_library -actions rate,sync-meta,link -filter unrated`

- Remove converted images and pp3s whose RAWs have been deleted and/or those with a low rating.

`photos -base my_library -actions cleanup -gt 2`

- Convert images with a rating > 2 and have been opened in rawtherapee (-edited) to jpegs

`photos -base my_library -actions convert -sizes 3840,1920,800 -filter normal -gt 2 -edited`

- Merge library `two` onto `one`

`rsync -ua two/ one`

## Install

`go get github.com/frizinak/photos/cmd/photos`

