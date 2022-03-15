package gorequests

import (
	"errors"
	"net/url"
	"os"
)

// handle URL params
func buildURLParams(userURL string, params map[string]string) (string, error) {
	Url, err := url.Parse(userURL)
	if err != nil {
		return "", errors.New(err.Error())
	}
	mkparams := url.Values{}
	for k, v := range params {
		mkparams.Set(k, v)

	}
	Url.RawQuery = mkparams.Encode()

	userURL = Url.String()
	return userURL, nil
}

func openFile(filename string) *os.File {
	r, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	return r
}
