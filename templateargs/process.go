package templateargs

import (
	"fmt"
	"strings"

	"github.com/yarpc/yab/templateargs/argparser"

	"gopkg.in/yaml.v2"
)

// ProcessMap takes a YAML request that may contain values like ${name:prashant}
// and replaces any template arguments with those specified in args.
func ProcessMap(req map[interface{}]interface{}, args map[string]string) (map[interface{}]interface{}, error) {
	argsUnmarshalled := make(map[string]interface{}, len(args))

	for k, v := range args {
		var unmarshalled interface{}
		if err := yaml.Unmarshal([]byte(v), &unmarshalled); err != nil {
			return nil, fmt.Errorf("failed to process %q with value %q: %v", k, v, err)
		}
		argsUnmarshalled[k] = unmarshalled
	}

	return processMap(req, argsUnmarshalled)
}

func processString(v string, args map[string]interface{}) (interface{}, error) {
	// If a string starts with \, avoid processing the value and strip the prefix.
	if strings.HasPrefix(v, `\`) {
		return v[1:], nil
	}

	// Anything that starts with "${ " should be processed as a template arg.
	if !strings.HasPrefix(v, "${") {
		return v, nil
	}

	parsed, err := argparser.Parse(v)
	if err != nil {
		return nil, err
	}

	// See if we have an argument for the specified variable.
	if replacement, ok := args[parsed.Name]; ok {
		return replacement, nil
	}

	if parsed.Value == "" {
		return "", nil
	}

	// Otherwise, unmarshal the value and return that.
	var unmarshalled interface{}
	err = yaml.Unmarshal([]byte(parsed.Value), &unmarshalled)
	return unmarshalled, err
}

func processValue(v interface{}, args map[string]interface{}) (interface{}, error) {
	switch v := v.(type) {
	case string:
		return processString(v, args)
	case map[interface{}]interface{}:
		return processMap(v, args)
	case []interface{}:
		return processList(v, args)
	default:
		return v, nil
	}

}

func processList(l []interface{}, args map[string]interface{}) ([]interface{}, error) {
	replacement := make([]interface{}, len(l))
	for i, v := range l {
		newV, err := processValue(v, args)
		if err != nil {
			return nil, err
		}
		replacement[i] = newV
	}

	return replacement, nil
}

func processMap(m map[interface{}]interface{}, args map[string]interface{}) (map[interface{}]interface{}, error) {
	replacement := make(map[interface{}]interface{}, len(m))
	for k, v := range m {
		newK, err := processValue(k, args)
		if err != nil {
			return nil, err
		}

		newV, err := processValue(v, args)
		if err != nil {
			return nil, err
		}

		replacement[newK] = newV
	}

	return replacement, nil
}
