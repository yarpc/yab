package plugin

import (
	"errors"
	"fmt"
	"testing"

	"go.uber.org/multierr"

	"github.com/stretchr/testify/assert"
)

type mockParser struct {
	flags                 []*flag
	shouldErr             bool
	errOnCallCount        int
	addFlagGroupCallCount int
}

func (p *mockParser) AddFlagGroup(short, long string, data interface{}) error {
	if p.shouldErr && p.errOnCallCount == p.addFlagGroupCallCount {
		return errors.New("bad")
	}
	p.flags = append(p.flags, &flag{
		groupName:       short,
		longDescription: long,
		data:            data,
	})
	p.addFlagGroupCallCount++
	return nil
}

type fooFlags struct {
	Bar string `long:"bar" description:"Sets the 'bar' value."`
}

// defer-able test cleanup for global state
func setFlags(flags []*flag) (restore func()) {
	_flags = flags
	return func() {
		_flags = nil
	}
}

func TestAddToParserSuccess(t *testing.T) {
	flags := []*flag{{}}
	defer setFlags(flags)()
	p := &mockParser{}

	err := AddToParser(p)
	assert.NoError(t, err)
	assert.Equal(t, 1, p.addFlagGroupCallCount)
	assert.Equal(t, 1, len(p.flags))
}

func TestAddToParserFail(t *testing.T) {
	flags := []*flag{{}}
	defer setFlags(flags)()
	p := &mockParser{
		shouldErr: true,
	}

	err := AddToParser(p)
	assert.Error(t, err)
	errs := multierr.Errors(err)
	assert.Equal(t, 1, len(errs))
	assert.Equal(t, 0, p.addFlagGroupCallCount)
	assert.Equal(t, 0, len(p.flags))
}

func TestAddToParserFailPartial(t *testing.T) {
	flags := []*flag{{}, {}, {}}
	defer setFlags(flags)()
	p := &mockParser{
		shouldErr:      true,
		errOnCallCount: 2,
	}

	err := AddToParser(p)
	assert.Error(t, err)
	errs := multierr.Errors(err)
	assert.Equal(t, 1, len(errs))
	assert.Equal(t, 2, p.addFlagGroupCallCount)
	assert.Equal(t, 2, len(p.flags))
}

func TestAddToParserMany(t *testing.T) {
	numFlags := 100
	mockFlags := make([]*flag, numFlags)
	for i := range mockFlags {
		mockFlags[i] = &flag{groupName: fmt.Sprintf("foo-%d", i)}
	}
	defer setFlags(mockFlags)()

	p := &mockParser{}

	errs := AddToParser(p)
	assert.NoError(t, errs)
	assert.Equal(t, numFlags, p.addFlagGroupCallCount)
	assert.Equal(t, numFlags, len(p.flags))

	for i := range mockFlags {
		assert.Equal(t, mockFlags[i], p.flags[i])
	}
}

func TestAddFlags(t *testing.T) {
	defer setFlags(nil)()
	short := "Foo Options"
	long := "This is a lot of usage information about Foo"
	foo := &fooFlags{}

	AddFlags(short, long, foo)
	assert.Equal(t, 1, len(_flags))

	setFlag := _flags[0]
	assert.Equal(t, short, setFlag.groupName)
	assert.Equal(t, long, setFlag.longDescription)
	assert.Equal(t, foo, setFlag.data)
}

func TestAddedFlagsArePassedToParser(t *testing.T) {
	defer setFlags(nil)()
	short := "Foo Options"
	long := "This is a lot of usage information about Foo"
	foo := &fooFlags{}

	p := &mockParser{}
	AddFlags(short, long, foo)
	errs := AddToParser(p)
	assert.NoError(t, errs)

	assert.Equal(t, 1, p.addFlagGroupCallCount)
	assert.Equal(t, 1, len(p.flags))

	setFlag := p.flags[0]
	assert.Equal(t, short, setFlag.groupName)
	assert.Equal(t, long, setFlag.longDescription)
	assert.Equal(t, foo, setFlag.data)
}
