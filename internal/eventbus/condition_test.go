package eventbus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConditions(t *testing.T) {
	got, err := ParseConditions([]string{"key=subject,op=in,value=a=b,value= c ", "key=zone,op=eq,value=is1b"})
	require.NoError(t, err)
	require.Equal(t, []TriggerCondition{
		{Key: "subject", Op: "in", Values: []string{"a=b", " c "}},
		{Key: "zone", Op: "eq", Values: []string{"is1b"}},
	}, got)
	for _, input := range []string{
		"", "key=zone", "op=eq,value=x", "key=zone,op=eq", "key=zone,op=eq,value=",
		"key=zone,op=eq,value=x,", "key=zone,op=eq,value=a,b",
		"key=zone,key=subject,op=eq,value=x", "key=zone,op=eq,op=in,value=x",
		"key=zone, op=eq,value=x", "unknown=x,key=zone,op=eq,value=x",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseConditions([]string{input})
			require.Error(t, err)
		})
	}
}
