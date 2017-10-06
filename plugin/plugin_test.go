package plugin

import (
	"errors"
	"fmt"
	"testing"

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
		shortDescription: short,
		longDescription:  long,
		data:             data,
	})
	p.addFlagGroupCallCount++
	return nil
}

type fooFlags struct {
	Bar string `long:"bar" description:"Sets the 'bar' value."`
}

// defer-able test cleanup for global state
func resetFlags(flags []*flag) func() {
	_flags = flags
	return func() {
		_flags = nil
	}
}

func TestAddToParser(t *testing.T) {
	tests := []struct {
		shouldErr                 bool
		errOnCallCount            int
		numErrs                   int
		numAddFlagGroupCallCounts int
		numParserFlags            int
	}{
		{numErrs: 0, numAddFlagGroupCallCounts: 3, numParserFlags: 3},
		{shouldErr: true, numErrs: 3},
		{shouldErr: true, errOnCallCount: 2, numErrs: 1, numAddFlagGroupCallCounts: 2, numParserFlags: 2},
	}

	threeFlags := []*flag{{}, {}, {}}
	defer resetFlags(threeFlags)()

	for _, tt := range tests {
		p := &mockParser{
			shouldErr:      tt.shouldErr,
			errOnCallCount: tt.errOnCallCount,
		}

		errs := AddToParser(p)
		if tt.shouldErr {
			assert.NotNil(t, errs)
		} else {
			assert.Nil(t, errs)
		}
		assert.Equal(t, tt.numErrs, len(errs))
		assert.Equal(t, tt.numAddFlagGroupCallCounts, p.addFlagGroupCallCount)
		assert.Equal(t, tt.numParserFlags, len(p.flags))
	}
}

func TestAddToParserMany(t *testing.T) {
	numFlags := 100
	mockFlags := make([]*flag, numFlags)
	for i := range mockFlags {
		mockFlags[i] = &flag{shortDescription: fmt.Sprintf("foo-%d", i)}
	}
	defer resetFlags(mockFlags)()

	p := &mockParser{}

	errs := AddToParser(p)
	assert.Nil(t, errs)
	assert.Equal(t, numFlags, p.addFlagGroupCallCount)
	assert.Equal(t, numFlags, len(p.flags))

	for i := range mockFlags {
		assert.Equal(t, mockFlags[i], p.flags[i])
	}
}

func TestAddFlags(t *testing.T) {
	defer resetFlags(nil)()
	short := "Foo Options"
	long := "This is a lot of usage information about Foo"
	foo := &fooFlags{}

	AddFlags(short, long, foo)
	assert.Equal(t, 1, len(_flags))

	setFlag := _flags[0]
	assert.Equal(t, short, setFlag.shortDescription)
	assert.Equal(t, long, setFlag.longDescription)
	assert.Equal(t, foo, setFlag.data)
}

func TestAddedFlagsArePassedToParser(t *testing.T) {
	defer resetFlags(nil)()
	short := "Foo Options"
	long := "This is a lot of usage information about Foo"
	foo := &fooFlags{}

	p := &mockParser{}
	AddFlags(short, long, foo)
	errs := AddToParser(p)
	assert.Nil(t, errs)

	assert.Equal(t, 1, p.addFlagGroupCallCount)
	assert.Equal(t, 1, len(p.flags))

	setFlag := p.flags[0]
	assert.Equal(t, short, setFlag.shortDescription)
	assert.Equal(t, long, setFlag.longDescription)
	assert.Equal(t, foo, setFlag.data)
}
