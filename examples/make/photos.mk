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
sync: cleanup
	$(PHOTOS_CMD) -filter all -actions link,sync-meta,link

## import: import photos using gphoto2 (camera or SD-card plugged in).
.PHONY: import
import: cleanup
	$(PHOTOS_CMD) -filter all -actions import,link,sync-meta,link,previews

## rate: rate/delete unrated images and sync.
.PHONY: rate
rate: cleanup
	$(PHOTOS_CMD) -filter unrated -actions rate
	$(PHOTOS_CMD) -filter all -actions link,sync-meta,link

## convert: convert all images where rating > $(RatingGT) to jpegs.
.PHONY: convert
convert: cleanup
	$(PHOTOS_CMD) -filter normal -gt $(RatingGT) -actions convert -sizes $(SIZES) -edited

## unedited: print list of links that should be edited in rawtherapee.
.PHONY: unedited
unedited:
	$(PHOTOS_CMD) -filter unedited -gt $(RatingGT) -actions show-links -no-raw

## cleanup: remove jpegs and pp3s of deleted files and
##          jpegs of images with rating not > $(RatingGT).
.PHONY: cleanup
cleanup:
	$(PHOTOS_CMD) -filter all -gt $(RatingGT) -actions cleanup -y

## gphotos: create flat folder hierarchy of all converted jpegs
##          (easy to upload to google photos).
.PHONY: gphotos
gphotos:
	rm -rf GPhotos
	mkdir GPhotos

	while read line; do\
		[[ $$line != *"/$(GPhotoSize)/"* ]] && continue;\
		ln -s "$$(realpath "$$line")" GPhotos/;\
	done < <($(PHOTOS_CMD) -filter normal -actions show-jpegs -no-raw)


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
import-quick: cleanup
	$(PHOTOS_CMD) -filter all -actions import,link,sync-meta,link

## previews: generate previews. e.g.: when laptop is plugged in.
.PHONY: previews
previews: cleanup
	$(PHOTOS_CMD) -filter all -actions previews

## plugged: do all the heavy lifting at night
.PHONY: plugged
plugged: sync previews convert
