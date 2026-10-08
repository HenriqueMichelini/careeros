package preprocessing

import "errors"

const PlanningVersion = "portions-v1"

type CapacityReason string

const (
	OriginalSizeCapacity CapacityReason = "original_size"
	SingleViewCapacity   CapacityReason = "single_view_capacity"
	IndivisibleCapacity  CapacityReason = "indivisible_or_context_capacity"
	PortionCountCapacity CapacityReason = "portion_count"
)

// Budget caps working text (including repeated context), the actual serialized
// provider payload, and portion count. All sizes are UTF-8 bytes, not tokens.
type Budget struct{ PreparedBytes, PayloadBytes, MaxPortions int }

// ContextReference explicitly labels repeated syntax; its ranges are overlaps,
// never newly covered source. Ancestors are ordered outermost first.
type ContextReference struct {
	Role       string
	SegmentID  string
	Text       string
	Normalized Range
	Original   []Range
}

type View struct {
	Text       string
	Normalized Range
	Original   []Range
	Context    []ContextReference
}

type Portion struct {
	View                        View
	PreparedBytes, PayloadBytes int
}

// Plan.Status is Ready only for a complete SINGLE view. Complete means only
// mechanical original coverage by the plan, not extraction completeness or
// permission to execute any portion. Unprocessed excludes context overlaps.
type Plan struct {
	SourceID, Version                                string
	Status                                           CapacityStatus
	Complete                                         bool
	OriginalBytes, PreparedBytes, SinglePayloadBytes int
	Portions                                         []Portion
	Unprocessed                                      []Range
	Reason                                           CapacityReason
}

// PayloadEncoder must be a pure serializer of the final provider request with
// all prompt/context/envelope overhead included. It must not call a provider.
// It is evaluated for candidate views; no monotonic size assumption is made.
type PayloadEncoder func(View) ([]byte, error)

func (s Source) Plan(b Budget, encode PayloadEncoder) (Plan, error) {
	if s.id == "" || s.version != StructureVersion {
		return Plan{}, errors.New("planning requires a prepared Source")
	}
	if b.PreparedBytes <= 0 || b.PayloadBytes <= 0 || b.MaxPortions <= 0 || encode == nil {
		return Plan{}, errors.New("positive byte/portion budgets and payload serializer required")
	}
	r := Plan{SourceID: s.id, Version: PlanningVersion, Status: InsufficientCapacity, OriginalBytes: len(s.original), PreparedBytes: len(s.text)}
	limits, _ := LimitsFor(s.field)
	if len(s.original) > limits.OriginalBytes {
		r.Unprocessed = []Range{{0, len(s.original)}}
		r.Reason = OriginalSizeCapacity
		return r, nil
	}
	view := View{Text: s.text, Normalized: Range{0, len(s.text)}}
	if len(s.text) > 0 {
		view.Original = []Range{{0, len(s.original)}}
	}
	p, err := measure(view, encode)
	if err != nil {
		return r, err
	}
	r.SinglePayloadBytes = p.PayloadBytes
	if fits(p, b) {
		r.Status, r.Complete = Ready, true
		r.Portions = []Portion{p}
		return r, nil
	}
	r.Reason = SingleViewCapacity
	if len(s.text) == 0 {
		return r, nil
	}
	units, forbidden := s.planningUnits()
	byID := make(map[string]Segment, len(s.segments))
	for _, seg := range s.segments {
		byID[seg.ID] = seg
	}
	var pending *Portion
	covered := 0
	flush := func() {
		if pending != nil {
			r.Portions = append(r.Portions, *pending)
			covered = pending.View.Normalized.End
			pending = nil
		}
	}
	fail := func(reason CapacityReason) (Plan, error) {
		r.Reason = reason
		remaining, err := s.Resolve(Range{covered, len(s.text)})
		r.Unprocessed = remaining
		return r, err
	}
	for _, unit := range units {
		start := unit.Start
		for start < unit.End {
			if len(r.Portions) >= b.MaxPortions {
				return fail(PortionCountCapacity)
			}
			from := start
			if pending != nil {
				from = pending.View.Normalized.Start
			}
			candidate, err := s.portion(Range{from, unit.End}, byID, encode)
			if err != nil {
				return Plan{}, err
			}
			if fits(candidate, b) {
				pending = &candidate
				break
			}
			if pending != nil {
				flush()
				continue
			}
			// The preferred complete unit does not fit. Try mapping boundaries
			// from largest to smallest, never entering a protected block/group.
			found := false
			for i := len(s.mapping) - 1; i >= 0; i-- {
				end := s.mapping[i].Normalized.End
				if end >= unit.End || end-start > b.PreparedBytes || forbidden[end] {
					continue
				}
				if end <= start {
					break
				}
				part, err := s.portion(Range{start, end}, byID, encode)
				if err != nil {
					return Plan{}, err
				}
				if fits(part, b) {
					pending = &part
					start = end
					found = true
					flush()
					break
				}
			}
			if !found {
				return fail(IndivisibleCapacity)
			}
		}
	}
	flush()
	r.Complete = true
	return r, nil
}

// Preferred units join wrapped prose, list continuations and protected runs.
// Safe fallback cuts are mapping boundaries outside lexical/protected blocks.
func (s Source) planningUnits() ([]Range, []bool) {
	guards := literalBytes(s.text)
	forbidden := make([]bool, len(s.text)+1)
	for i := 1; i < len(s.text); i++ {
		forbidden[i] = guards[i-1] && guards[i]
	}
	var units []Range
	for i, seg := range s.segments {
		protected := seg.Kind == Literal || seg.Kind == Table || seg.Kind == Quotation || seg.Kind == Heading || seg.Kind == ListItem
		if protected {
			for j := seg.Normalized.Start + 1; j < seg.Normalized.End; j++ {
				forbidden[j] = true
			}
		}
		join := false
		if i > 0 {
			prev := s.segments[i-1]
			join = forbidden[seg.Normalized.Start] ||
				(seg.Kind == Text && prev.Kind == Text && seg.ParentID == prev.ParentID) ||
				(seg.Kind == ListContinuation && (prev.Kind == ListItem || prev.Kind == ListContinuation)) ||
				(seg.Kind == prev.Kind && (seg.Kind == Literal || seg.Kind == Table || seg.Kind == Quotation))
			if join && protected && seg.Kind == prev.Kind && seg.Kind != Heading && seg.Kind != ListItem {
				forbidden[seg.Normalized.Start] = true
			}
		}
		if join {
			units[len(units)-1].End = seg.Normalized.End
		} else {
			units = append(units, seg.Normalized)
		}
	}
	return units, forbidden
}

func (s Source) portion(r Range, byID map[string]Segment, encode PayloadEncoder) (Portion, error) {
	original, err := s.Resolve(r)
	if err != nil {
		return Portion{}, err
	}
	v := View{Text: s.text[r.Start:r.End], Normalized: r, Original: original}
	// Find the segment containing the first covered byte. Parent references
	// already form an earlier-only chain; ancestors within this view are covered
	// content, while earlier ancestors are explicit repeated context overlaps.
	lo, hi := 0, len(s.segments)
	for lo < hi {
		mid := (lo + hi) / 2
		if s.segments[mid].Normalized.End <= r.Start {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	parent := s.segments[lo].ParentID
	for parent != "" {
		seg := byID[parent]
		if seg.Normalized.End <= r.Start {
			v.Context = append(v.Context, ContextReference{"context", seg.ID, s.text[seg.Normalized.Start:seg.Normalized.End], seg.Normalized, append([]Range(nil), seg.Original...)})
		}
		parent = seg.ParentID
	}
	for i, j := 0, len(v.Context)-1; i < j; i, j = i+1, j-1 {
		v.Context[i], v.Context[j] = v.Context[j], v.Context[i]
	}
	return measure(v, encode)
}

func measure(v View, encode PayloadEncoder) (Portion, error) {
	data, err := encode(v)
	n := len(v.Text)
	for _, c := range v.Context {
		n += len(c.Text)
	}
	return Portion{v, n, len(data)}, err
}

func fits(p Portion, b Budget) bool {
	return p.PreparedBytes <= b.PreparedBytes && p.PayloadBytes <= b.PayloadBytes
}
