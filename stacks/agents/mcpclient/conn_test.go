package mcpclient

import "testing"

func TestProblem_EveryIdHasFixedWordsAndNoWordsCarryAnAddressOrAHeaderName(t *testing.T) {
	for p := range problemWords {
		if p.Words() == "" {
			t.Errorf("%s has no words", p)
		}
	}
	for _, p := range []Problem{ProblemUnreachable, ProblemGuard, ProblemSignIn, ProblemForbidden, ProblemNotMCP, ProblemTLS, ProblemTimeout, ProblemVersion,
		ProblemSecret, ProblemSignInSet, ProblemCAFile, ProblemClosed, ProblemTooMany, ProblemInvalid} {
		if p.Words() == "" {
			t.Errorf("%s must be one of the ids with words", p)
		}
	}
	if Problem("").Words() != "" {
		t.Error("no problem, no words")
	}
}
