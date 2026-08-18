package e2e

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func decodeYAMLDocuments(data []byte) ([]runtime.Object, error) {
	decoder := codecFactory.UniversalDeserializer()
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	var out []runtime.Object
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 || isCommentOnlyYAML(doc) {
			continue
		}
		obj, _, err := decoder.Decode(doc, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("decode document: %w", err)
		}
		out = append(out, obj)
	}
	return out, nil
}

func isCommentOnlyYAML(doc []byte) bool {
	hasContent := false
	for line := range bytes.SplitSeq(doc, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		hasContent = true
		if line[0] != '#' {
			return false
		}
	}
	return hasContent
}
