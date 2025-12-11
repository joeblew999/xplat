package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// JqCmd provides jq-compatible JSON processing
var JqCmd = &cobra.Command{
	Use:   "jq <query> [file]",
	Short: "Process JSON with jq syntax",
	Long: `Process JSON using jq query syntax (powered by gojq).

Reads JSON from file or stdin, applies the query, and outputs results.

Examples:
  xplat jq '.name' package.json
  echo '{"foo":"bar"}' | xplat jq '.foo'
  xplat jq '.assets[].name' < releases.json
  xplat jq -r '.version' package.json

Common queries:
  .              Identity (pretty-print)
  .foo           Get field
  .foo.bar       Nested field
  .[]            Iterate array
  .[0]           Array index
  .foo[]         Iterate array field
  select(.x > 1) Filter
  keys           Object keys
  length         Array/string length`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runJq,
}

var (
	jqRaw    bool
	jqSlurp  bool
	jqNull   bool
)

func init() {
	JqCmd.Flags().BoolVarP(&jqRaw, "raw-output", "r", false, "Output raw strings without quotes")
	JqCmd.Flags().BoolVarP(&jqSlurp, "slurp", "s", false, "Read entire input into array")
	JqCmd.Flags().BoolVarP(&jqNull, "null-input", "n", false, "Don't read input, use null")
}

func runJq(cmd *cobra.Command, args []string) error {
	queryStr := args[0]

	// Parse the query
	query, err := gojq.Parse(queryStr)
	if err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}

	// Compile the query
	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	// Determine input source
	var input io.Reader
	if len(args) > 1 {
		f, err := os.Open(args[1])
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}
		defer f.Close()
		input = f
	} else {
		input = os.Stdin
	}

	// Handle null input
	if jqNull {
		return runQuery(code, nil)
	}

	// Handle slurp mode (read all into array)
	if jqSlurp {
		var inputs []interface{}
		decoder := json.NewDecoder(input)
		for {
			var v interface{}
			if err := decoder.Decode(&v); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("invalid JSON: %w", err)
			}
			inputs = append(inputs, v)
		}
		return runQuery(code, inputs)
	}

	// Process each JSON value from input
	decoder := json.NewDecoder(bufio.NewReader(input))
	for {
		var v interface{}
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("invalid JSON: %w", err)
		}
		if err := runQuery(code, v); err != nil {
			return err
		}
	}

	return nil
}

func runQuery(code *gojq.Code, input interface{}) error {
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return err
		}
		if err := outputValue(v); err != nil {
			return err
		}
	}
	return nil
}

func outputValue(v interface{}) error {
	if v == nil {
		fmt.Println("null")
		return nil
	}

	// Raw output for strings
	if jqRaw {
		if s, ok := v.(string); ok {
			fmt.Println(s)
			return nil
		}
	}

	// Pretty-print JSON
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode output: %w", err)
	}
	fmt.Println(string(output))
	return nil
}
