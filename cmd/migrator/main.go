// Copyright 2017 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: javasgl	songganglin@gmail.com
package main

import (
	"sander/cmd"
	"sander/config"
	"sander/logger"
)

func main() {
	logger.Init(config.ROOT + "/log/migrator")
	server.MigratorServer()
}
