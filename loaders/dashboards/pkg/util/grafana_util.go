// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"encoding/hex"
	"hash/fnv"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"k8s.io/klog"
)

const (
	defaultAdmin = "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
)

// GenerateUID generates UID for customized dashboard
func GenerateUID(namespace string, name string) (string, error) {
	uid := namespace + "-" + name
	if len(uid) > 40 {
		hasher := fnv.New128a()
		_, err := hasher.Write([]byte(uid))
		if err != nil {
			return "", err
		}
		uid = hex.EncodeToString(hasher.Sum(nil))
	}
	return uid, nil
}

// GetHTTPClient returns http client
func getHTTPClient() *http.Client {
	transport := &http.Transport{}
	client := &http.Client{Transport: transport}
	return client
}

// SetRequest ...
func SetRequest(method string, url string, body io.Reader, retry int) ([]byte, int) {
	req, _ := http.NewRequest(method, url, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", defaultAdmin)

	resp, err := getHTTPClient().Do(req)
	times := 0
	for {
		if err == nil {
			break
		}
		klog.Error("failed to send HTTP request. Retry in 5 seconds ", "error ", err)
		time.Sleep(time.Second * 5)
		times++
		if times == retry {
			klog.Errorf("failed to send HTTP request after retrying %v times", retry)
			break
		}
		resp, err = getHTTPClient().Do(req)
	}

	if resp != nil {
		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			klog.Info("failed to parse response body ", "error ", err)
		}
		return respBody, resp.StatusCode
	} else {
		return nil, http.StatusNotFound
	}
}
