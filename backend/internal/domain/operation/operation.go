package operation

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
)

// ErrAlreadyFinished は終端状態の操作を変更しようとしたことを表す。
var ErrAlreadyFinished = errors.New("操作は既に終了しています")

// ErrNoSteps はステップの無い操作を作ろうとしたことを表す。
var ErrNoSteps = errors.New("操作には少なくとも 1 つのステップが必要です")

// ErrStepOverflow は総ステップ数を超えて進めようとしたことを表す。
var ErrStepOverflow = errors.New("これ以上ステップを進められません")

// Snapshot は操作のある時点の完全な状態。
//
// イベントに毎回同梱する。1 つ取りこぼしても表示がずれないようにするため、
// 差分ではなく全体を持つ。
type Snapshot struct {
	ID           ID
	Kind         Kind
	State        State
	StartedAt    time.Time
	FinishedAt   time.Time
	StepIndex    int
	StepTotal    int
	CurrentStep  string
	StepNames    []string
	BytesDone    int64
	BytesTotal   int64
	ErrorCode    string
	ErrorMessage string
	Attributes   map[string]string
}

// Event は進捗の 1 件。
type Event struct {
	seq      int64
	at       time.Time
	level    Level
	message  string
	snapshot Snapshot
}

// Seq は操作内での通し番号を返す。1 始まりで単調増加する。
func (e Event) Seq() int64 { return e.seq }

// At は発生時刻を返す。
func (e Event) At() time.Time { return e.at }

// Level は深刻度を返す。
func (e Event) Level() Level { return e.level }

// Message はログの本文を返す。
func (e Event) Message() string { return e.message }

// Snapshot はこの時点の操作の完全な状態を返す。
func (e Event) Snapshot() Snapshot { return e.snapshot }

// Operation は 1 回の操作。
type Operation struct {
	id         ID
	kind       Kind
	state      State
	startedAt  time.Time
	finishedAt time.Time

	stepNames []string
	stepIndex int

	bytesDone  int64
	bytesTotal int64

	errorCode    string
	errorMessage string

	attributes map[string]string

	events  []Event
	nextSeq int64
}

// New は操作を作る。作った時点では StatePending。
func New(id ID, kind Kind, stepNames []string, at time.Time) (*Operation, error) {
	if !id.IsValid() {
		return nil, ErrInvalidID
	}
	if len(stepNames) == 0 {
		return nil, ErrNoSteps
	}

	return &Operation{
		id:         id,
		kind:       kind,
		state:      StatePending,
		startedAt:  at,
		stepNames:  slices.Clone(stepNames),
		attributes: map[string]string{},
		nextSeq:    1,
	}, nil
}

// ID は識別子を返す。
func (o *Operation) ID() ID { return o.id }

// Kind は種類を返す。
func (o *Operation) Kind() Kind { return o.kind }

// State は状態を返す。
func (o *Operation) State() State { return o.state }

// StartedAt は開始時刻を返す。
func (o *Operation) StartedAt() time.Time { return o.startedAt }

// FinishedAt は終了時刻を返す。未完了ならゼロ値。
func (o *Operation) FinishedAt() time.Time { return o.finishedAt }

// StepIndex は現在のステップ番号（1 始まり）を返す。開始前は 0。
func (o *Operation) StepIndex() int { return o.stepIndex }

// StepTotal は総ステップ数を返す。
func (o *Operation) StepTotal() int { return len(o.stepNames) }

// CurrentStep は現在のステップ名を返す。開始前は空文字。
func (o *Operation) CurrentStep() string {
	if o.stepIndex < 1 || o.stepIndex > len(o.stepNames) {
		return ""
	}
	return o.stepNames[o.stepIndex-1]
}

// Bytes は処理済みと総バイト数を返す。総量が不明なら total は 0。
func (o *Operation) Bytes() (done, total int64) { return o.bytesDone, o.bytesTotal }

// ErrorCode は失敗時の安定した識別子を返す。
func (o *Operation) ErrorCode() string { return o.errorCode }

// ErrorMessage は失敗時の説明を返す。画面にそのまま出る。
func (o *Operation) ErrorMessage() string { return o.errorMessage }

// IsFinished は終端状態かを返す。
func (o *Operation) IsFinished() bool { return o.state.IsTerminal() }

// Attributes は操作固有の情報を返す。呼び出し側が変更しても影響しない。
func (o *Operation) Attributes() map[string]string { return maps.Clone(o.attributes) }

// Events は進捗イベントの一覧を返す。呼び出し側が変更しても影響しない。
func (o *Operation) Events() []Event { return slices.Clone(o.events) }

// Advance は次のステップへ進める。最初の呼び出しで StateRunning になる。
func (o *Operation) Advance(at time.Time) error {
	if o.state.IsTerminal() {
		return fmt.Errorf("%w（%s）", ErrAlreadyFinished, o.state)
	}
	if o.stepIndex >= len(o.stepNames) {
		return fmt.Errorf("%w（%d/%d）", ErrStepOverflow, o.stepIndex, len(o.stepNames))
	}

	o.state = StateRunning
	o.stepIndex++
	o.record(at, LevelInfo, o.CurrentStep())
	return nil
}

// Log は進捗ログを記録する。状態は変えない。
func (o *Operation) Log(at time.Time, level Level, message string) {
	o.record(at, level, message)
}

// SetBytes は処理済みと総バイト数を更新する。
func (o *Operation) SetBytes(at time.Time, done, total int64) {
	o.bytesDone = done
	o.bytesTotal = total
	o.record(at, LevelInfo, "")
}

// SetAttribute は操作固有の情報を記録する。
func (o *Operation) SetAttribute(key, value string) {
	o.attributes[key] = value
}

// Succeed は成功として終了する。
func (o *Operation) Succeed(at time.Time) error {
	if o.state.IsTerminal() {
		return fmt.Errorf("%w（%s）", ErrAlreadyFinished, o.state)
	}
	o.state = StateSucceeded
	o.finishedAt = at
	o.record(at, LevelInfo, "完了しました")
	return nil
}

// Fail は失敗として終了する。
//
// code はログや条件分岐に使う安定した識別子、message は画面に出す説明。
func (o *Operation) Fail(at time.Time, code, message string) error {
	if o.state.IsTerminal() {
		return fmt.Errorf("%w（%s）", ErrAlreadyFinished, o.state)
	}
	o.state = StateFailed
	o.finishedAt = at
	o.errorCode = code
	o.errorMessage = message
	o.record(at, LevelError, message)
	return nil
}

// record はイベントを 1 件追加する。
// snapshot はその時点の値を固定するため、後から操作が進んでも変わらない。
func (o *Operation) record(at time.Time, level Level, message string) {
	o.events = append(o.events, Event{
		seq:      o.nextSeq,
		at:       at,
		level:    level,
		message:  message,
		snapshot: o.snapshot(),
	})
	o.nextSeq++
}

func (o *Operation) snapshot() Snapshot {
	return Snapshot{
		ID:           o.id,
		Kind:         o.kind,
		State:        o.state,
		StartedAt:    o.startedAt,
		FinishedAt:   o.finishedAt,
		StepIndex:    o.stepIndex,
		StepTotal:    len(o.stepNames),
		CurrentStep:  o.CurrentStep(),
		StepNames:    slices.Clone(o.stepNames),
		BytesDone:    o.bytesDone,
		BytesTotal:   o.bytesTotal,
		ErrorCode:    o.errorCode,
		ErrorMessage: o.errorMessage,
		Attributes:   maps.Clone(o.attributes),
	}
}
