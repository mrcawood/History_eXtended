package ingest

import (
	"fmt"

	"github.com/history-extended/hx/internal/spool"
	"github.com/history-extended/hx/internal/store"
)

// Run reads events from spool, pairs pre+post, inserts into DB.
// Idempotent: INSERT OR IGNORE on events.
func Run(st *store.Store, eventsPath string) (int, error) {
	events, err := spool.Read(eventsPath)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	// Buffer pre events by (sid, seq)
	preBuf := make(map[string]*store.PreEvent)
	var inserted int

	for _, e := range events {
		if e.T == "pre" {
			key := fmt.Sprintf("%s:%d", e.Sid, e.Seq)
			preBuf[key] = &store.PreEvent{
				T:    e.T,
				Ts:   e.Ts,
				Sid:  e.Sid,
				Seq:  e.Seq,
				Cmd:  e.Cmd,
				Cwd:  e.Cwd,
				Tty:  e.Tty,
				Host: e.Host,
			}
			continue
		}
		// post
		key := fmt.Sprintf("%s:%d", e.Sid, e.Seq)
		pre, ok := preBuf[key]
		if !ok {
			continue
		}
		delete(preBuf, key)

		if err := st.EnsureSession(pre.Sid, pre.Host, pre.Tty, pre.Cwd, pre.Ts); err != nil {
			continue
		}
		cmdID, err := st.CmdID(pre.Cmd, pre.Ts)
		if err != nil {
			continue
		}
		post := &store.PostEvent{
			T:     e.T,
			Ts:    e.Ts,
			Sid:   e.Sid,
			Seq:   e.Seq,
			Exit:  e.Exit,
			DurMs: e.DurMs,
			Pipe:  e.Pipe,
		}
		insertedRow, ierr := st.InsertEvent(pre, post, cmdID)
		if ierr != nil {
			continue
		}
		if insertedRow {
			inserted++
		}
		if err := st.UpdateSessionEnded(pre.Sid, post.Ts); err != nil {
			// non-fatal
		}
	}

	return inserted, nil
}
