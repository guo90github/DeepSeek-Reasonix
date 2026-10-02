package board

import "context"

// Ops returns the log's ops in order. A caller that needs provenance — who asked
// for a node, who refuted one — reads them here, because the folded state keeps
// only current facts.
func (b *Board) Ops(ctx context.Context) ([]Op, error) {
	release, err := b.lockShared(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	read, err := readLog(b.logPath)
	if err != nil {
		return nil, err
	}
	return read.Ops, nil
}
