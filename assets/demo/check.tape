# Website clip: zap check and its exit code, then the schedule status.
# Never `zap schedule on` here: it would install a real launchd agent or systemd timer.
Output check.mp4
Output check.gif
Require zap
Set FontSize 16
Set Width 1200
Set Height 420
Set Padding 24
# Size first: vhs ignores Set commands that come after a Source.
Source settings.tape

Hide
Type "cd /tmp/zap-demo/home/dev/my-app && clear" Enter
Show
Type "zap check; echo exit code $?" Sleep 300ms Enter
Sleep 4s
Type "zap schedule status" Sleep 300ms Enter
Sleep 2.5s
