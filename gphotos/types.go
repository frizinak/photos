package gphotos

import (
	"encoding/json"
	"fmt"
	"time"
)

type BatchCreate struct {
	AlbumID       string         `json:"albumId,omitempty"`
	NewMediaItems []*MediaItem   `json:"newMediaItems"`
	AlbumPosition *AlbumPosition `json:"albumPosition,omitempty"`
}

type MediaItem struct {
	Description     string          `json:"description"`
	SimpleMediaItem SimpleMediaItem `json:"simpleMediaItem"`
}

type SimpleMediaItem struct {
	UploadToken string `json:"uploadToken"`
	Filename    string `json:"fileName"`
}

type AlbumPosition struct {
	// todo
}

type BatchCreateResult struct {
	NewMediaItems []CreateMediaItemResult `json:"newMediaItemResults"`
}

type BatchGetResult struct {
	MediaItemResults []MediaItemResult `json:"mediaItemResults"`
}

type MediaItemResult struct {
	MediaItem RealMediaItemResult `json:"mediaItem"`
}

type CreateMediaItemResult struct {
	MediaItemResult
	UploadToken string       `json:"uploadToken"`
	Status      StatusResult `json:"status"`
}

type StatusResult struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

type RealMediaItemResult struct {
	ID              string                `json:"id"`
	Description     string                `json:"description"`
	ProductURL      string                `json:"productUrl"`
	BaseURL         string                `json:"baseUrl"`
	Mime            string                `json:"mimeType"`
	MediaMetaData   MediaMetaDataResult   `json:"mediaMetadata"`
	ContributorInfo ContributorInfoResult `json:"contributorInfo"`
	Filename        string                `json:"filename"`
}

type MediaMetaDataResult struct {
	CreationTime time.Time    `json:"creationTime"`
	Width        string       `json:"width"`
	Height       string       `json:"height"`
	Photo        *PhotoResult `json:"photo"`
	Video        *VideoResult `json:"video"`
}

type PhotoResult struct {
	CameraMake  string  `json:"cameraMake"`
	CameraModel string  `json:"cameraModel"`
	FocalLength float64 `json:"focalLength"`
	ApertureF   float64 `json:"apertureFNumber"`
	ISO         int     `json:"isoEquivalent"`
	Exposure    string  `json:"exposureTime"`
}

type VideoResult struct {
	CameraMake  string  `json:"cameraMake"`
	CameraModel string  `json:"cameraModel"`
	FPS         float64 `json:"fps"`
	STatus      string  `json:"status"`
}

type ContributorInfoResult struct {
	ProfilePictureBaseURL string `json:"profilePictureBaseUrl"`
	DisplayName           string `json:"displayName"`
}

func (b BatchCreateResult) Err() error {
	for _, i := range b.NewMediaItems {
		if err := i.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (mi RealMediaItemResult) String() string {
	return fmt.Sprintf("%s %s", mi.ID, mi.Filename)
}

func (i CreateMediaItemResult) Err() error {
	err := i.Status.Err()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", err, i.MediaItem)
}

func (s StatusResult) Err() error {
	if s.Code == 0 {
		return nil
	}

	return fmt.Errorf(
		"google error: %d: %s\ndetails: %s",
		s.Code,
		s.Message,
		string(s.Details),
	)
}
