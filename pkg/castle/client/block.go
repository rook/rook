package client

import (
	"bytes"
	"encoding/json"
	"path"

	"github.com/quantum/castle/pkg/model"
)

const (
	ImageQueryName        = "image"
	ImageMapInfoQueryName = "mapinfo"
)

func (c *CastleNetworkRestClient) GetBlockImages() ([]model.BlockImage, error) {
	body, err := c.DoGet(ImageQueryName)
	if err != nil {
		return nil, err
	}

	var images []model.BlockImage
	err = json.Unmarshal(body, &images)
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (c *CastleNetworkRestClient) CreateBlockImage(newImage model.BlockImage) (string, error) {
	body, err := json.Marshal(newImage)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(ImageQueryName, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *CastleNetworkRestClient) GetBlockImageMapInfo() (model.BlockImageMapInfo, error) {
	body, err := c.DoGet(path.Join(ImageQueryName, ImageMapInfoQueryName))
	if err != nil {
		return model.BlockImageMapInfo{}, err
	}

	var imageMapInfo model.BlockImageMapInfo
	err = json.Unmarshal(body, &imageMapInfo)
	if err != nil {
		return model.BlockImageMapInfo{}, err
	}

	return imageMapInfo, nil
}
