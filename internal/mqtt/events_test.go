package mqtt

import "testing"

func TestTransitionTable(t *testing.T) {
	all := []ConnState{StateDisconnected, StateConnecting, StateConnected, StateReconnecting, StateFailed}
	for _, s := range all {
		if _, ok := transitions[s]; !ok {
			t.Errorf("state %s has no transition row", s)
		}
	}

	legal := []struct{ from, to ConnState }{
		{StateDisconnected, StateConnecting},
		{StateConnecting, StateConnected},
		{StateConnecting, StateReconnecting}, // first attempt can fail and be retried
		{StateConnecting, StateFailed},
		{StateConnected, StateReconnecting},
		{StateReconnecting, StateReconnecting}, // repeated failures update the error
		{StateReconnecting, StateConnected},
		{StateFailed, StateConnecting},
		{StateConnected, StateDisconnected},
		{StateReconnecting, StateDisconnected},
		{StateFailed, StateDisconnected},
	}
	for _, c := range legal {
		if !c.from.CanTransitionTo(c.to) {
			t.Errorf("%s -> %s should be legal", c.from, c.to)
		}
	}

	illegal := []struct{ from, to ConnState }{
		{StateDisconnected, StateConnected},
		{StateDisconnected, StateDisconnected},
		{StateDisconnected, StateReconnecting},
		{StateConnected, StateConnecting},
		{StateFailed, StateConnected},
		{StateFailed, StateReconnecting},
	}
	for _, c := range illegal {
		if c.from.CanTransitionTo(c.to) {
			t.Errorf("%s -> %s should be illegal", c.from, c.to)
		}
	}
}

func TestLive(t *testing.T) {
	for s, want := range map[ConnState]bool{
		StateDisconnected: false, StateConnecting: true, StateConnected: true,
		StateReconnecting: true, StateFailed: false,
	} {
		if got := s.Live(); got != want {
			t.Errorf("%s.Live() = %v, want %v", s, got, want)
		}
	}
}
