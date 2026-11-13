# Website clip: zap scan --all over the demo repositories.
Output scan.mp4
Output scan.gif
Require zap
Set FontSize 16
Set Width 1200
Set Height 640
Set Padding 24
# Size first: vhs ignores Set commands that come after a Source.
Source settings.tape

Hide
Type "cd /tmp/zap-demo/home/dev/my-app && clear" Enter
Show
Type "zap scan --all" Sleep 300ms Enter
Sleep 5s
