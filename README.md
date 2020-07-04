# RAW Photo library manager

## Dependencies

imagemagick
gphoto2
ffmpeg for .MOVs
rawtherapee to use `-actions convert`

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

`photos -base my_library -actions rate -filter unrated,sync-meta`

- Remove converted images and pp3s whose RAWs have been deleted.

`photos -base my_library -actions cleanup`

- Convert images with a rating > 2 to jpegs

`photos -base my_library -actions convert -sizes 3840,1920,800 -filter normal -gt 2``

## Install

`go get github.com/frizinak/photos/cmd/photos`

