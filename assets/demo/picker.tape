# Website clip: the interactive picker. Details for a worktree, review, zap.
# Keys assume the fixture order (see zap.tape).
Output picker.mp4
Output picker.gif
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
Type "zap --all" Sleep 300ms Enter
Sleep 2.5s
Down Sleep 400ms Down Sleep 400ms Down Sleep 400ms Down Sleep 700ms
Type "e" Sleep 3.5s
Escape Sleep 700ms
Up Sleep 300ms Up Sleep 300ms Up Sleep 300ms Up Sleep 700ms
Enter Sleep 3s
Type "y" Sleep 5s
