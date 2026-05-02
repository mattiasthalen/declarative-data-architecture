// prism/internal/events/events_test.go
package events

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	in := `{"event":"source.start","source":"adventure_works"}
{"event":"entity.start","entity":"Customer"}
not-json garbage line
{"event":"entity.end","entity":"Customer","rows":12,"load_id":"L1","files":2}
`
	var got []Event
	err := Parse(strings.NewReader(in), func(e Event) error {
		got = append(got, e)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, "source.start", got[0].Event)
	assert.Equal(t, "Customer", got[1].Entity)
	assert.Equal(t, "runner.warn", got[2].Event)
	assert.Equal(t, "not-json garbage line", got[2].Message)
	assert.Equal(t, 12, got[3].Rows)
}
