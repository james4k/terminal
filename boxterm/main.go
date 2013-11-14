package main

import (
	"bufio"
	"fmt"
	"io"
	//"math/rand"
	"os"
	"os/exec"

	"github.com/james4k/termbox-go-noinput"
	"j4k.co/terminal"
)

func filterESC(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		br := bufio.NewReader(r)
		for {
			b, err := br.ReadByte()
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			if b == 033 {
				_, err = pw.Write([]byte("ESC"))
			} else {
				_, err = pw.Write([]byte{b})
			}
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}
	}()
	return pr
}

func update(term *terminal.VT, state *terminal.State, w, h int) {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	for i := 0; i < h+2; i++ {
		termbox.SetCell(0, i, '│', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(w+1, i, '│', termbox.ColorDefault, termbox.ColorDefault)
	}
	for i := 0; i < w+2; i++ {
		termbox.SetCell(i, 0, '─', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(i, h+1, '─', termbox.ColorDefault, termbox.ColorDefault)
	}
	termbox.SetCell(0, 0, '┌', termbox.ColorDefault, termbox.ColorDefault)
	termbox.SetCell(w+1, 0, '┐', termbox.ColorDefault, termbox.ColorDefault)
	termbox.SetCell(w+1, h+1, '┘', termbox.ColorDefault, termbox.ColorDefault)
	termbox.SetCell(0, h+1, '└', termbox.ColorDefault, termbox.ColorDefault)

	state.Lock()
	defer state.Unlock()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c, fg, bg := state.Cell(x, y)
			/*
				// termbox only supports 8 colors
				if fg > 15 {
					fg = 7
				} else if fg > 7 {
					fg -= 8
				}
				if bg > 15 {
					bg = 0
				} else if bg > 7 {
					bg -= 8
				}
			*/
			fg = 6 // colors are an issue for later; just keep it monocolored for now
			bg = 0
			termbox.SetCell(x+1, y+1, c,
				termbox.Attribute(fg+1),
				termbox.Attribute(bg+1))
		}
	}
	if state.CursorVisible() {
		curx, cury := state.Cursor()
		curx += 1
		cury += 1
		termbox.SetCursor(curx, cury)
	} else {
		termbox.SetCursor(-1, -1)
	}
	termbox.Flush()
}

func logpanic() {
	if x := recover(); x != nil {
		fmt.Fprintln(os.Stderr, x)
	}
}

func main() {
	// NOTE: This must be before termbox.Init(). On OSX, at least, we get a
	// kernel panic if we termbox.Init() first! But, only when the process is
	// terminated in some way. Crazy. If this was more than a debug app it
	// might be worth looking more into.
	var state terminal.State
	cmd := exec.Command(os.Getenv("SHELL"), "-i")
	term, pty, err := terminal.Start(&state, cmd)
	if err != nil {
		panic(err)
	}
	defer term.Close()

	err = termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()
	wide, tall := termbox.Size()

	term.Resize(wide-2, tall-2)
	// TODO: separate window for the log output
	term.Write([]byte("boxterm - debug frontend\r\n"))

	endc := make(chan bool)
	updatec := make(chan bool, 1)
	go func() {
		defer logpanic()
		for {
			err := term.Parse()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				break
			}
			select {
			case updatec <- true:
			default:
			}
		}
		close(endc)
	}()

	go func() {
		defer logpanic()
		io.Copy(pty, os.Stdin)
	}()

	eventc := make(chan termbox.Event, 4)
	go func() {
		for {
			eventc <- termbox.PollEvent()
		}
	}()

	for {
		select {
		case ev := <-eventc:
			if ev.Type == termbox.EventResize {
				wide = ev.Width
				tall = ev.Height
				term.Resize(wide-2, tall-2)
			}
		case <-endc:
			return
		case <-updatec:
			update(term, &state, wide-2, tall-2)
		}
	}
}
