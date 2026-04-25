package metadata

import (
	"context"
	"fmt"

	"github.com/gpustack/gguf-parser-go"
	"llm-proxy/models"
)

// GGUFScanner provides high-performance GGUF metadata parsing.
type GGUFScanner struct{}

func NewGGUFScanner() *GGUFScanner {
	return &GGUFScanner{}
}

// Scan extracts metadata from a GGUF file without loading the entire file into RAM.
func (s *GGUFScanner) Scan(ctx context.Context, path string) (models.ModelMetadata, error) {
	// Respect context before starting
	select {
	case <-ctx.Done():
		return models.ModelMetadata{}, ctx.Err()
	default:
	}

	// Use gguf-parser-go to inspect the file
	// Parsing the header only is sufficient for metadata
	gf, err := gguf_parser.ParseGGUFFile(path)
	if err != nil {
		return models.ModelMetadata{}, fmt.Errorf("failed to parse GGUF: %w", err)
	}

	gm := gf.Metadata()
	ga := gf.Architecture()

	metadata := models.ModelMetadata{
		Name:          gm.Name,
		Architecture:  gm.Architecture,
		ContextLength: int(ga.MaximumContextLength),
		Parameters:    int64(gf.ModelParameters),
		Author:        gm.Author,
		Description:   gm.Description,
	}

	// Quantization logic: Use FileTypeDescriptor (e.g. Q4_K_M) as it's more descriptive for users.
	metadata.Quantization = gm.FileTypeDescriptor
	if metadata.Quantization == "" && gm.QuantizationVersion > 0 {
		metadata.Quantization = fmt.Sprintf("v%d", gm.QuantizationVersion)
	}

	return metadata, nil
}
