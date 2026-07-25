package controller

import (
	"errors"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOAuthSession struct {
	values  map[interface{}]interface{}
	saveErr error
}

func newFakeOAuthSession() *fakeOAuthSession {
	return &fakeOAuthSession{values: map[interface{}]interface{}{}}
}

func (session *fakeOAuthSession) ID() string {
	return "test-session"
}

func (session *fakeOAuthSession) Get(key interface{}) interface{} {
	return session.values[key]
}

func (session *fakeOAuthSession) Set(key interface{}, value interface{}) {
	session.values[key] = value
}

func (session *fakeOAuthSession) Delete(key interface{}) {
	delete(session.values, key)
}

func (session *fakeOAuthSession) Clear() {
	session.values = map[interface{}]interface{}{}
}

func (session *fakeOAuthSession) AddFlash(value interface{}, vars ...string) {}

func (session *fakeOAuthSession) Flashes(vars ...string) []interface{} {
	return nil
}

func (session *fakeOAuthSession) Options(options sessions.Options) {}

func (session *fakeOAuthSession) Save() error {
	return session.saveErr
}

func TestConsumeOAuthStateKeepsReferralBoundToExactState(t *testing.T) {
	session := newFakeOAuthSession()
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, saveOAuthStates(session, map[string]oauthStateEntry{
		"first-state": {
			AffCode:   "first-aff",
			CreatedAt: now.Unix(),
		},
		"second-state": {
			AffCode:   "second-aff",
			CreatedAt: now.Unix(),
		},
	}))

	affCode, ok, err := consumeOAuthState(session, "first-state", now)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "first-aff", affCode)

	remaining := loadOAuthStates(session)
	assert.NotContains(t, remaining, "first-state")
	assert.Equal(t, "second-aff", remaining["second-state"].AffCode)

	_, ok, err = consumeOAuthState(session, "first-state", now)
	require.NoError(t, err)
	assert.False(t, ok)

	affCode, ok, err = consumeOAuthState(session, "second-state", now)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "second-aff", affCode)
}

func TestConsumeOAuthStateRejectsExpiredState(t *testing.T) {
	session := newFakeOAuthSession()
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, saveOAuthStates(session, map[string]oauthStateEntry{
		"expired-state": {
			AffCode:   "stale-aff",
			CreatedAt: now.Add(-oauthStateTTL - time.Second).Unix(),
		},
	}))

	affCode, ok, err := consumeOAuthState(session, "expired-state", now)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "stale-aff", affCode)
	assert.NotContains(t, loadOAuthStates(session), "expired-state")
}

func TestPruneOAuthStatesBoundsSessionGrowth(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := map[string]oauthStateEntry{
		"expired": {
			CreatedAt: now.Add(-oauthStateTTL - time.Second).Unix(),
		},
	}
	for index := 0; index < oauthStateLimit; index++ {
		states[string(rune('a'+index))] = oauthStateEntry{
			CreatedAt: now.Add(time.Duration(index) * time.Second).Unix(),
		}
	}

	pruneOAuthStates(states, now)
	assert.NotContains(t, states, "expired")
	assert.Len(t, states, oauthStateLimit-1)
	assert.NotContains(t, states, "a")
}

func TestConsumeOAuthStatePropagatesSessionSaveFailure(t *testing.T) {
	session := newFakeOAuthSession()
	session.saveErr = errors.New("save failed")

	_, ok, err := consumeOAuthState(session, "missing", time.Now())
	require.Error(t, err)
	assert.False(t, ok)
}
