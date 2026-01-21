// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package http

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"unicode/utf8"
)

type tokenGetter func() string

type bearerRoundTripper struct {
	getToken tokenGetter
	wrapper  http.RoundTripper
}

func NewBearerRoundTripper(token tokenGetter, rt http.RoundTripper) http.RoundTripper {
	return &bearerRoundTripper{getToken: token, wrapper: rt}
}

func (rt *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", rt.getToken()))
	return rt.wrapper.RoundTrip(req)
}

type debugRoundTripper struct {
	next   http.RoundTripper
	logger *slog.Logger
}

func NewDebugRoundTripper(logger *slog.Logger, next http.RoundTripper) *debugRoundTripper {
	return &debugRoundTripper{next, logger.With("component", "http/debugroundtripper")}
}

func (rt *debugRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	reqd, _ := httputil.DumpRequest(req, false)
	reqBody := bodyToString(&req.Body)

	res, err = rt.next.RoundTrip(req)
	if err != nil {
		rt.logger.Error("err", "error", err)
		return
	}

	resd, _ := httputil.DumpResponse(res, false)
	resBody := bodyToString(&res.Body)

	rt.logger.Debug("round trip", "url", req.URL,
		"requestdump", string(reqd), "requestbody", reqBody,
		"responsedump", string(resd), "responsebody", resBody)
	return
}

func bodyToString(body *io.ReadCloser) string {
	if *body == nil {
		return "<nil>"
	}

	var b bytes.Buffer
	_, err := b.ReadFrom(*body)
	if err != nil {
		panic(err)
	}
	if err = (*body).Close(); err != nil {
		panic(err)
	}
	*body = io.NopCloser(&b)

	s := b.String()
	if utf8.ValidString(s) {
		return s
	}

	return hex.Dump(b.Bytes())
}
