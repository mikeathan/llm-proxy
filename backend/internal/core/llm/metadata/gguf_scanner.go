package metadata

import (
	"context"
	"fmt"

	gguf_parser "github.com/gpustack/gguf-parser-go"
	"llm-proxy/models"
)

// GGUFScanner provides high-performance GGUF metadata parsing.
type GGUFScanner struct{}

func NewGGUFScanner() *GGUFScanner {
	return &GGUFScanner{}
}

// Scan extracts metadata from a GGUF file without loading the entire file into RAM.
// It uses SkipLargeMetadata and UseMMap to read only the file header (a few KB),
// making it instantaneous even for multi-GB models.
func (s *GGUFScanner) Scan(ctx context.Context, path string) (models.ModelMetadata, error) {
	select {
	case <-ctx.Done():
		return models.ModelMetadata{}, ctx.Err()
	default:
	}

	gf, err := gguf_parser.ParseGGUFFile(path,
		gguf_parser.SkipLargeMetadata(), // Skip tensor/large metadata blobs — header only
		gguf_parser.UseMMap(),           // mmap instead of read() — OS handles page faults
	)
	if err != nil {
		return models.ModelMetadata{}, fmt.Errorf("failed to parse GGUF: %w", err)
	}

	gm := gf.Metadata()
	ga := gf.Architecture()

	meta := models.ModelMetadata{
		Name:          gm.Name,
		Architecture:  gm.Architecture,
		ContextLength: int(ga.MaximumContextLength),
		Parameters:    int64(gf.ModelParameters),
		Author:        gm.Author,
		Description:   gm.Description,
	}

	meta.Quantization = gm.FileTypeDescriptor
	if meta.Quantization == "" && gm.QuantizationVersion > 0 {
		meta.Quantization = fmt.Sprintf("v%d", gm.QuantizationVersion)
	}

	return meta, nil
}
