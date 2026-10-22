package alerting

import (
	"sync"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// State of one (rule, machine) pair, modelled on Prometheus alerting:
//
//	inactive --breach--> pending --breach held for `For`--> firing
//	    ^                   |                                  |
//	    +----no breach------+------------no breach-------------+ (emits Resolved)
//
// The `For` duration suppresses one-off spikes: a value must stay out of
// range that long before anyone is paged.
type State int

const (
	Inactive State = iota
	Pending
	Firing
)

func (s State) String() string { return [...]string{"inactive", "pending", "firing"}[s] }

type EventKind string

const (
	Fired    EventKind = "firing"
	Resolved EventKind = "resolved"
)

// Event is emitted when a (rule, machine) pair starts firing or resolves.
type Event struct {
	Kind    EventKind
	Rule    Rule
	Machine string
	Value   float64
	At      time.Time
	Since   time.Time // when the breach began
}

type pairKey struct {
	ruleID  int64
	machine string
}

type pairState struct {
	state State
	since time.Time
}

// Evaluator holds the active rule set and per-machine state. It is pure: no
// I/O, and time comes from the readings themselves, so replaying a stream
// always produces the same events. Safe for concurrent use.
type Evaluator struct {
	mu       sync.Mutex
	byTenant map[string][]Rule
	states   map[pairKey]*pairState
}

func NewEvaluator() *Evaluator {
	return &Evaluator{byTenant: map[string][]Rule{}, states: map[pairKey]*pairState{}}
}

// SetRules replaces the rule set. State survives for rules that still exist,
// so a periodic reload never re-pages anyone. Rules removed or disabled while
// firing produce Resolved events.
func (e *Evaluator) SetRules(rules []Rule, now time.Time) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	active := make(map[int64]Rule, len(rules))
	e.byTenant = map[string][]Rule{}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		active[r.ID] = r
		e.byTenant[r.Tenant] = append(e.byTenant[r.Tenant], r)
	}

	var events []Event
	for k, st := range e.states {
		if _, ok := active[k.ruleID]; ok {
			continue
		}
		if st.state == Firing {
			events = append(events, Event{Kind: Resolved, Rule: Rule{ID: k.ruleID}, Machine: k.machine, At: now, Since: st.since})
		}
		delete(e.states, k)
	}
	return events
}

// Observe feeds one reading through every matching rule.
func (e *Evaluator) Observe(r telemetry.Reading) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	at := r.Time()
	var events []Event
	for _, rule := range e.byTenant[r.Tenant] {
		value, ok := r.Values[rule.Metric]
		if !ok || !rule.AppliesTo(r.Machine) {
			continue
		}
		if ev, fired := e.step(rule, r.Machine, value, at); fired {
			events = append(events, ev)
		}
	}
	return events
}

// step advances the state machine of one pair and reports an event if the
// transition produced one.
func (e *Evaluator) step(rule Rule, machine string, value float64, at time.Time) (Event, bool) {
	k := pairKey{rule.ID, machine}
	st := e.states[k]
	if st == nil {
		st = &pairState{}
		e.states[k] = st
	}
	ev := Event{Rule: rule, Machine: machine, Value: value, At: at}
	breached := rule.Operator.Breached(value, rule.Threshold)

	switch {
	case breached && st.state == Inactive:
		st.state, st.since = Pending, at
		if rule.For == 0 {
			st.state = Firing
			ev.Kind, ev.Since = Fired, at
			return ev, true
		}
	case breached && st.state == Pending && at.Sub(st.since) >= rule.For:
		st.state = Firing
		ev.Kind, ev.Since = Fired, st.since
		return ev, true
	case !breached && st.state == Firing:
		st.state = Inactive
		ev.Kind, ev.Since = Resolved, st.since
		return ev, true
	case !breached:
		st.state = Inactive
	}
	return Event{}, false
}

// StateOf exposes the current state of a pair, for tests and diagnostics.
func (e *Evaluator) StateOf(ruleID int64, machine string) State {
	e.mu.Lock()
	defer e.mu.Unlock()
	if st := e.states[pairKey{ruleID, machine}]; st != nil {
		return st.state
	}
	return Inactive
}
