package codetool

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// OpenImageParams are the parameters for the open_image tool.
type OpenImageParams struct {
	Path string `json:"path" jsonschema:"description=Path to the image file to view (PNG\\, JPEG\\, GIF\\, WebP)"`
}

const maxImageBytes = 20 * 1024 * 1024 // 20MB

// OpenImage creates a tool that reads an image file and returns it as a
// multimodal tool result so the model can see its contents visually.
// Supports PNG, JPEG, GIF, and WebP formats.
func OpenImage(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[OpenImageParams](
		"open_image",
		"View an image file. Returns the image so you can see its contents visually. "+
			"Supports PNG, JPEG, GIF, and WebP. Use this to inspect rendered plots, "+
			"diagrams, screenshots, or any visual output you've generated.",
		func(ctx context.Context, params OpenImageParams) (core.ToolResultWithImages, error) {
			if params.Path == "" {
				return core.ToolResultWithImages{}, &core.ModelRetryError{Message: "path must not be empty"}
			}
			path := params.Path
			if !filepath.IsAbs(path) && cfg.WorkDir != "" {
				path = filepath.Join(cfg.WorkDir, path)
			}
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return core.ToolResultWithImages{}, &core.ModelRetryError{Message: "file not found: " + params.Path}
				}
				return core.ToolResultWithImages{}, fmt.Errorf("stat file: %w", err)
			}
			if info.Size() > maxImageBytes {
				return core.ToolResultWithImages{}, &core.ModelRetryError{
					Message: fmt.Sprintf("image too large (%d bytes, max %d)", info.Size(), maxImageBytes),
				}
			}

			// Detect MIME type from extension.
			ext := strings.ToLower(filepath.Ext(path))
			mimeType := ""
			switch ext {
			case ".png":
				mimeType = "image/png"
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".gif":
				mimeType = "image/gif"
			case ".webp":
				mimeType = "image/webp"
			default:
				return core.ToolResultWithImages{}, &core.ModelRetryError{
					Message: "unsupported image format: " + ext + ". Supported: png, jpg, gif, webp",
				}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return core.ToolResultWithImages{}, fmt.Errorf("read image: %w", err)
			}

			dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))

			return core.ToolResultWithImages{
				Text: fmt.Sprintf("Image: %s (%d bytes, %s)", filepath.Base(path), len(data), mimeType),
				Images: []core.ImagePart{
					{URL: dataURL, MIMEType: mimeType},
				},
			}, nil
		},
	)
}
