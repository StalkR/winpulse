* `-ldflags -H=windowsgui` avoids console Window but then no logging in `cmd.exe`
  find a way to keep it

* detect Windows audio settings, and adjust format accordingly, so that we
  don't need:

  * right-click on sound in systray
  * choose tab "playback",
  * choose "speakers" and click "properties"
  * tab "advanced" default format should be "16 bit, 44100 Hz (CD Quality)"

* make port optional

* implement PulseAudio native protocol
