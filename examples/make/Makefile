SHELL := /bin/bash

# camera timezone offset in minutes
# e.g.: daylight saving time always off in brussels: TIMEZONE 120
# e.g.: daylight saving time always on             : TIMEZONE 60
TIMEZONE := 0

# base command
PHOTOS_CMD := photos -base . -tz $(TIMEZONE)


# list of jpeg sizes
SIZES := 3840,1920

# size to upload to Google Photos (one of SIZES)
GPhotoSize := 3840

# size to symlink to Flag (one of SIZES)
FlatSize := 3840

# only images with a rating > $(RatingGT) will be converted
RatingGT := 2

include photos.mk
