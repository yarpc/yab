package yamlalias

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func stringPtr(s string) *string {
	return &s
}

func TestWrap(t *testing.T) {
	type Simple struct {
		Foo string `yaml:"foo" yaml-aliases:"bar,baz"`
	}

	type ManyTypes struct {
		Str   string            `yaml-aliases:"string"`
		Int   int               `yaml-aliases:"Int"`
		Slice []string          `yaml-aliases:"list"`
		Map   map[string]string `yaml-aliases:"dict"`
	}

	type AliasToPtr struct {
		Foo *string `yaml:"foo" yaml-aliases:"bar"`
	}

	tests := []struct {
		msg  string
		data string
		want interface{}
	}{
		{
			msg:  "No aliases",
			data: `{"foo": "foo", "bar": "bar"}`,
			want: &struct {
				Foo string
				Bar string
			}{"foo", "bar"},
		},
		{
			msg:  "set field by normal name",
			data: `{"foo": "foo"}`,
			want: &Simple{"foo"},
		},
		{
			msg:  "set field by first alias",
			data: `{"bar": "bar"}`,
			want: &Simple{"bar"},
		},
		{
			msg:  "set field by second alias",
			data: `{"baz": "baz"}`,
			want: &Simple{"baz"},
		},
		{
			msg:  "set field with multiple aliases, last one wins",
			data: `{"bar": "bar", "foo": "foo", "baz": "baz"}`,
			want: &Simple{"baz"},
		},
		{
			msg:  "pointer field that is not set",
			data: `{}`,
			want: &AliasToPtr{},
		},
		{
			msg:  "pointer field set directly",
			data: `{"foo": "foo"}`,
			want: &AliasToPtr{stringPtr("foo")},
		},
		{
			msg:  "pointer field that is set by alias",
			data: `{"bar": "bar"}`,
			want: &AliasToPtr{stringPtr("bar")},
		},
		{
			msg: "Unexported fields are ignored",
			want: &struct {
				Foo string `yaml:"foo" yaml-aliases:"bar"`
				bar string `yaml:"bar" yaml-aliases:"baz"`
			}{
				Foo: "bar",
				bar: "",
			},
			data: `{"bar": "bar"}`,
		},
		{
			msg: "Alias with different case",
			want: &struct {
				Foo string `yaml:"foo" yaml-aliases:"Foo"`
			}{
				Foo: "Foo",
			},
			data: `{"Foo": "Foo"}`,
		},
		{
			msg: "Alias with dashes",
			want: &struct {
				Foo string `yaml:"foo" yaml-aliases:"foo-bar"`
			}{
				Foo: "foo-bar",
			},
			data: `{"foo-bar": "foo-bar"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			// To allow use of anonymous structs, we use reflection to create
			// a new empty struct.
			v := reflect.New(reflect.TypeOf(tt.want).Elem()).Interface()
			require.NoError(t, Unmarshal([]byte(tt.data), v))
			assert.Equal(t, tt.want, v)
		})
	}
}

func TestWrapPointerField(t *testing.T) {
	var dest string
	type StructWithPointer struct {
		Foo *string `yaml:"foo" yaml-aliases:"bar"`
	}

	s := StructWithPointer{Foo: &dest}
	require.NoError(t, yaml.Unmarshal([]byte(`{"foo": "initial"}`), &s), "failed to unmarshal")
	assert.Equal(t, "initial", dest, "dest expected to be updated")

	require.NoError(t, Unmarshal([]byte(`{"bar": "updated"}`), &s), "failed to unmarshal with alias")
	assert.Equal(t, "updated", dest, "expected dest to be updated via alias")
}

func TestWrapInvalid(t *testing.T) {
	tests := []struct {
		msg string
		t   interface{}
	}{
		{
			msg: "Alias clashes with other field",
			t: &struct {
				Foo string `yaml:"foo" yaml-aliases:"bar,baz"`
				Bar string `yaml:"bar"`
			}{},
		},
		{
			msg: "Alias clashes with another alias",
			t: &struct {
				Foo string `yaml:"foo" yaml-aliases:"baz"`
				Bar string `yaml:"bar" yaml-aliases:"baz"`
			}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.Panics(t, func() {
				Unmarshal([]byte(`{}`), &tt.t)
			})
		})
	}
}
