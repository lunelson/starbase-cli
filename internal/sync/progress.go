package sync

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Progress tracks and displays sync operation progress
type Progress struct {
	out   io.Writer
	isTTY bool

	mu      sync.Mutex
	total   int
	done    int
	ok      int
	errors  int
	last    string
	phase   string
	stopped bool

	ticker *time.Ticker
	stopCh chan struct{}
	wg     sync.WaitGroup
	frame  int
}

// NewProgress creates a progress reporter
func NewProgress(out io.Writer) *Progress {
	fd := -1
	if f, ok := out.(*os.File); ok {
		fd = int(f.Fd())
	}

	return &Progress{
		out:    out,
		isTTY:  fd >= 0 && term.IsTerminal(fd),
		stopCh: make(chan struct{}),
	}
}

// IsTTY returns whether output is a terminal
func (p *Progress) IsTTY() bool {
	return p.isTTY
}

// StartGitOps begins tracking git operations
func (p *Progress) StartGitOps(total int) {
	p.mu.Lock()
	p.total = total
	p.done = 0
	p.ok = 0
	p.errors = 0
	p.phase = "Git ops"
	p.stopped = false
	p.mu.Unlock()

	p.ticker = time.NewTicker(100 * time.Millisecond)
	p.wg.Add(1)
	go p.run()
}

// UpdateGitOp records completion of a git operation
func (p *Progress) UpdateGitOp(name string, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.done++
	p.last = name
	if success {
		p.ok++
	} else {
		p.errors++
	}
}

// StopGitOps ends git operation tracking
func (p *Progress) StopGitOps() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopCh)
	p.wg.Wait()

	if p.isTTY {
		fmt.Fprintf(p.out, "\r\033[K")
	}
}

func (p *Progress) run() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ticker.C:
			p.render()
		case <-p.stopCh:
			return
		}
	}
}

func (p *Progress) render() {
	p.mu.Lock()
	defer p.mu.Unlock()

	spinner := spinnerFrames[p.frame%len(spinnerFrames)]
	p.frame++

	if p.isTTY {
		line := fmt.Sprintf("\r%s %s: %d/%d done (ok: %d, err: %d)",
			spinner, p.phase, p.done, p.total, p.ok, p.errors)
		if p.last != "" {
			line += fmt.Sprintf(" — %s", p.last)
		}
		fmt.Fprintf(p.out, "\033[K%s", line)
	}
}

// FetchPage prints progress for API pagination
func (p *Progress) FetchPage(page, totalSoFar int) {
	if p.isTTY {
		fmt.Fprintf(p.out, "\r\033[K  Fetching page %d... (%d repos so far)", page, totalSoFar)
	}
}

// FetchPageDone prints completion of a page fetch
func (p *Progress) FetchPageDone(page, fetched, totalSoFar int) {
	if p.isTTY {
		fmt.Fprintf(p.out, "\r\033[K")
	}
}

// FetchComplete prints final fetch count
func (p *Progress) FetchComplete(total int) {
	if p.isTTY {
		fmt.Fprintf(p.out, "\r\033[K")
	}
	fmt.Fprintf(p.out, "Found %d starred repos\n", total)
}
