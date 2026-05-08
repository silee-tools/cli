package static

import "embed"

//go:embed index.html app.js style.css pages/*
var Assets embed.FS
