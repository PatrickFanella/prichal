package enrollment

import (
	"fmt"
	"testing"
)

func TestMemberAdministrationNeedsBothLocalOptIns(t *testing.T) {
	for _, entry := range []struct {
		value                            string
		local, collaboration, want, fail bool
	}{
		{"", true, true, false, false}, {"false", false, false, false, false}, {"true", true, true, true, false},
		{"true", false, true, false, true}, {"true", true, false, false, true}, {"yes", true, true, false, true},
	} {
		t.Run(fmt.Sprintf("%s/local=%t/collaboration=%t", entry.value, entry.local, entry.collaboration), func(t *testing.T) {
			t.Setenv("PDS_LOCAL_MEMBER_ADMINISTRATION", entry.value)
			got, err := LoadMemberAdministration(entry.local, entry.collaboration)
			if got != entry.want || (err != nil) != entry.fail {
				t.Fatalf("permission %v, error %v", got, err)
			}
		})
	}
}
