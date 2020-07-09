own_path := $(abspath $(lastword $(MAKEFILE_LIST)))
.PHONY: help
help:
	@echo
	@echo "photos.mk help:"
	@cat $(own_path) | grep '^##'

##
## Default flow:
## Lazy, just need backups:
##   - `make import`
## Reviewing vibe:
##   - `make previews rate`
## Need them jpegs asap:
##   - optionally check output of `make unedited` and edit ./Collection in rawtherapee
##   - `make convert`
##

## sync: create/delete symlinks and update ratings/deleted flags.
.PHONY: sync
sync:
	$(PHOTOS_CMD) -action link,sync-meta,link

## import: import photos using gphoto2 (camera or SD-card plugged in).
.PHONY: import
import:
	$(PHOTOS_CMD) -action import,link,sync-meta,link,previews

## rate: rate/delete unrated images and sync.
.PHONY: rate
rate:
	$(PHOTOS_CMD) -filter undeleted,unrated -action rate
	$(PHOTOS_CMD) -action link,sync-meta,link

## tag: tag/rate/delete untagged images and sync.
.PHONY: tag
tag:
	$(PHOTOS_CMD) -filter undeleted -action rate -gt $(RatingGT) -tag '-'
	$(PHOTOS_CMD) -action link,sync-meta,link

## convert: convert all images where rating > $(RatingGT) to jpegs.
.PHONY: convert
convert:
	$(PHOTOS_CMD) -filter undeleted,edited -gt $(RatingGT) -action convert -sizes $(SIZES)

## unedited: print list of links that should be edited in rawtherapee.
.PHONY: unedited
unedited:
	$(PHOTOS_CMD) -filter undeleted,unedited -gt $(RatingGT) -action show-links -no-raw

## cleanup: remove jpegs and pp3s of deleted files and
##          jpegs of images with rating not > $(RatingGT).
.PHONY: cleanup
cleanup:
	$(PHOTOS_CMD) -gt $(RatingGT) -action sync-meta
	$(PHOTOS_CMD) -gt $(RatingGT) -action cleanup

## gphotos: create flat folder hierarchy of all converted jpegs
##          (easy to upload to google photos).
.PHONY: gphotos
gphotos:
	rm -rf GPhotos
	mkdir GPhotos

	while read line; do\
		[[ $$line != *"/$(GPhotoSize)/"* ]] && continue;\
		ln -s "$$(realpath "$$line")" GPhotos/;\
	done < <($(PHOTOS_CMD) -filter undeleted -gt $(RatingGT) -action show-jpegs -no-raw)

## gvideos: converts videos with h264_nvenc (nvidia) to ./GVideos
##          (easy to upload to google photos).
.PHONY: gvideos
gvideos:
	@-mkdir GVideos >/dev/null 2>&1

	while read line; do\
		[[ "$$line" != *".MOV" ]] && continue;\
		dst="GVideos/$$(basename "$$line" .MOV).mp4";\
		[ -e "$$dst" ] && continue;\
		tmp="$$dst.tmp";\
		ffmpeg -loglevel warning \
		-hwaccel:v auto -y \
		-i "$$line" \
		-c:a aac \
		-c:v h264_nvenc -rc vbr -preset llhp -cq 30 \
		-f mp4\
		"$$tmp" </dev/null &&\
		mv "$$tmp" "$$dst"; \
	done < <($(PHOTOS_CMD) -filter undeleted -action show-links -no-raw)

##
##
## Traveling / battery-saving workflow alternatives:
## e.g.:
## On the road:
##   - snap some pics
##   - camera is getting full / want backup on laptop: `make import-quick`
##   - repeat
## Laptop plugged in (at night):
##   - generate previews and convert edited: `make plugged`
## Feel like reviewing some images:
##   - `make previews`
##   - `make rate`
##

## import-quick: lightweight version of import.
.PHONY: import-quick
import-quick:
	$(PHOTOS_CMD) -action import,link,sync-meta,link

## previews: generate previews. e.g.: when laptop is plugged in.
.PHONY: previews
previews:
	$(PHOTOS_CMD) -action previews

## plugged: do all the heavy lifting at night
.PHONY: plugged
plugged: sync previews convert
