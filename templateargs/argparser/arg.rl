package argparser

import "fmt"


%%{
    machine envvar;
    write data;
}%%

// Var is the parsed envvar string.
type Var struct {
  Name  string
  Value string
}

// Parse parses strings in the format, ${NAME} and ${NAME:VALUE}.
func Parse(data string) (Var, error) {
    var (
        // Ragel variables
        cs   = 0
        p    = 0
        pe   = len(data)

        // variable name
        nameStart = -1
        nameEnd   = -1

        // default value
        valueStart = -1
        valueEnd   = -1
    )

    %%{
        name
            = ([a-zA-Z_] ([a-zA-Z0-9_] | '.' [a-zA-Z0-9_])*)
            >{ nameStart = fpc }
            @{ nameEnd = fpc + 1 }
            ;

        value
            = (any - '}')+
            >{ valueStart = fpc }
            @{ valueEnd = fpc + 1 }
            ;

        main := '${' name (':' value)? '}';

        write init;
        write exec;
    }%%

    if cs < %%{ write first_final; }%% {
        return Var{}, fmt.Errorf("%q is not in the form ${NAME} or ${NAME:DEFAULT}. e.g., ${name:John Smith}", data)
    }

    name := data[nameStart:nameEnd]
    var value string
    if (valueStart > 0) {
        value = data[valueStart:valueEnd]
    }

    return Var{Name: name, Value: value}, nil
}
