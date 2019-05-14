// Package yamlalias adds support for aliases when parsing YAML.
//
// Aliases are specified as a comma-separated list specified via
// "yaml-aliases" struct tag.
//
// Example:
//
//   type Config struct {
//     UserName string `yaml:"username" yaml-aliases:"userName,user-name"`
//   }
//
// This will parse the following YAML examples in the same way:
//   {"username": "John Doe"}
//   {"userName": "John Doe"}
//   {"user-name": "John Doe"}
//
// Note: It only adds aliases for top-level fields in the struct.
package yamlalias

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
)

// Unmarshal is a wrapper for yaml.Unmarshal but adds
// support for any yaml-aliases specified in out.
func Unmarshal(in []byte, out interface{}) error {
	return yaml.Unmarshal(in, addAliases(out))
}

// UnmarshalStrict is a wrapper for yaml.UnmarshalStrict but adds
// support for any yaml-aliases specified in out.
func UnmarshalStrict(in []byte, out interface{}) error {
	return yaml.UnmarshalStrict(in, addAliases(out))
}

// addAliases generates a new struct that contains all exported fields
// from the passed in struct, but adds additional fields for any
// aliases.
func addAliases(v interface{}) interface{} {
	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()

	fields := make([]reflect.StructField, 0, rt.NumField())
	dests := make([]reflect.Value, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)

		// Ignore unexported fields.
		if f.PkgPath != "" {
			continue
		}

		// Add a pointer field that matches the original field in the struct.
		fields = append(fields, reflect.StructField{
			Name: f.Name,
			Type: reflect.PtrTo(f.Type),
			Tag:  f.Tag,
		})
		dests = append(dests, rv.Field(i).Addr())

		allAliases, ok := f.Tag.Lookup("yaml-aliases")
		if !ok {
			// No aliases specified. No other fields to add.
			continue
		}

		aliases := strings.Split(allAliases, ",")
		for j, alias := range aliases {
			// Add a new field that is a pointer to the original field.
			// We generate a name that is unlikely to clash. Clashes will cause panics.
			fields = append(fields, reflect.StructField{
				Name: fmt.Sprintf("%vYamlAlias%v", f.Name, j),
				Type: reflect.PtrTo(f.Type),
				Tag:  reflect.StructTag(fmt.Sprintf(`yaml:%q`, alias)),
			})
			dests = append(dests, rv.Field(i).Addr())
		}
	}

	// The struct we generated has pointers to the original fields.
	// Set all the pointers to the fields in the original struct.
	generated := reflect.StructOf(fields)
	generatedV := reflect.New(generated)
	for i := range fields {
		generatedV.Elem().Field(i).Set(dests[i])
	}

	return generatedV.Interface()
}
