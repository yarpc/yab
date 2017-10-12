package plugin

import (
	"fmt"

	"go.uber.org/multierr"
)

// AddFlags should be used by embedded modules to inject custom flags into `yab`.
// It adds a set of custom flags with heading `groupName`.
//
// The `data` argument should be a pointer to a struct with one field for each
// command line flag. Each field should use tags like `description` to inform
// the parser of metadata about the flag. (tags are the same as supported by
// github.com/jessevdk/go-flags)
//
// 	type foo struct {
// 		Bar string `long:"bar" description:"Sets the 'bar' value."`
// 	}
//
//	AddFlags("Foo Options", "", &foo{})
//
// This would inject a custom flag group that looks like:
//
//	Foo Options
// 		--bar	Sets the 'bar' value.
//
// In order to retrieve the mutated results of the set flag, users of AddFlags() should
// retain a reference to the `data` object and check its values after parsing is complete.
func AddFlags(groupName string, longDescription string, data interface{}) {
	_flags = append(_flags, &flag{
		groupName:       groupName,
		longDescription: longDescription,
		data:            data,
	})
}

// Parser is any object that can add flag groups to itself before performing its parse.
type Parser interface {
	// AddFlagGroup adds an additional flag group to process during parsing.
	AddFlagGroup(groupName, longDescription string, data interface{}) error
}

// AddToParser adds all registered flags to the passed Parser.
// This operation is not atomic, flags are applied on a best-effort basis (not "all-or-nothing")
// Returns a slice of errors indicating which flags groups failed to be added.
func AddToParser(p Parser) error {
	var err error
	for _, f := range _flags {
		flagErr := p.AddFlagGroup(f.groupName, f.longDescription, f.data)
		if flagErr != nil {
			err = multierr.Append(err, fmt.Errorf("adding %v to parser: %v", f.groupName, flagErr))
		}
	}
	return err
}

// stores the list of currently registered flags
var _flags []*flag

type flag struct {
	groupName       string
	longDescription string
	data            interface{}
}
