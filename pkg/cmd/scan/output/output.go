package output

import (
	"encoding/json"
	"fmt"

	"github.com/snyk/driftctl/pkg/analyser"
)

type FormatOutput func(analysis *analyser.Analysis) ([]byte, error)

func GetFormatter(format string) (FormatOutput, error) {
	switch format {
	case "console":
		return WriteConsole, nil
	case "json":
		return func(analysis *analyser.Analysis) ([]byte, error) {
			return json.MarshalIndent(analysis, "", "\t")
		}, nil
	case "html":
		return WriteHTML, nil
	case "plan":
		return WritePlan, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
