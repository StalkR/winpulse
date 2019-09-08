# WinPulse

[![Build Status][1]][2] [![Godoc][3]][4]

WinPulse captures Audio from Windows and streams it to a PulseAudio Server.

It can stream with native protocol or over SSH with pacat.
It shows in systray with PulseAudio icon, right-click to stop,
play or exit.

Build with:
`go get -ldflags -H=windowsgui -trimpath github.com/StalkR/winpulse`

*Build flags `-ldflags -H=windowsgui` are to avoid a console Window
since it is a systray app.*

*Build flag `-trimpath` [new in Go 1.13][6] removes all file system
paths from the compiled executable, to improve build reproducibility.

Bugs, comments, questions: create a [new issue][5].

[1]: https://api.travis-ci.org/StalkR/winpulse.png?branch=master
[2]: https://travis-ci.org/StalkR/winpulse
[3]: https://godoc.org/github.com/StalkR/winpulse?status.png
[4]: https://godoc.org/github.com/StalkR/winpulse
[5]: https://github.com/StalkR/winpulse/issues/new
[6]: https://golang.org/doc/go1.13#go-command
