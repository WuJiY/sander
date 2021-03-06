// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of captchaHandler source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: polaris	polaris@studygolang.com

package controller

import (
	xhttp "sander/http"

	"github.com/dchest/captcha"
	"github.com/labstack/echo"
)

var captchaHandler = captcha.Server(100, 40)

// 验证码
type CaptchaController struct{}

func (c CaptchaController) RegisterRoute(g *echo.Group) {
	g.Get("/captcha/*", c.Server)
}

func (CaptchaController) Server(ctx echo.Context) error {
	captchaHandler.ServeHTTP(xhttp.ResponseWriter(ctx), xhttp.Request(ctx))
	return nil
}
