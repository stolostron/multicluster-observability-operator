package reader

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedString string
	}{
		{
			name:           "Read full strings",
			input:          "Hello",
			expectedString: "Hello",
		},
		{
			name:           "Cut the excess and drop the rest",
			input:          "Hello world",
			expectedString: "Hello wo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LimitedReader{R: strings.NewReader(tt.input), N: 8}
			b := make([]byte, 8)
			for {
				_, err := r.Read(b)
				if err == io.EOF {
					break
				} else if err == ErrTooLong {
					break
				}
			}

			if strings.Compare(string(bytes.Trim(b, "\x00")), tt.expectedString) != 0 {
				t.Errorf("%v is not equal to the expected: %v", string(b), tt.expectedString)
			}
		})
	}
}
