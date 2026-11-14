# Wiki screenshots of the picker: the list, a worktree's details, the review, the result.
# Shorter than wiki.tape, so the list sits close to the key bar. Record with wiki.sh.
Output wiki/wiki-picker.gif
Require zap
# Recorded at twice the size, and shown at half width, so text is sharp on high-DPI screens.
Set FontSize 32
Set Width 2800
Set Height 1120
Set Padding 56
Source settings.tape

Hide
Type "cd /tmp/zap-demo/home/dev/my-app && clear" Enter
Show
Type "zap --all" Enter Sleep 3s
Screenshot wiki/shots/picker.png
Sleep 1s
Down Sleep 400ms Down Sleep 400ms Down Sleep 400ms Down Sleep 700ms
Type "e" Sleep 3s
Screenshot wiki/shots/picker-details.png
Sleep 1s
Escape Sleep 700ms
Up Sleep 300ms Up Sleep 300ms Up Sleep 300ms Up Sleep 700ms
Type "e" Sleep 3s
Screenshot wiki/shots/picker-kept.png
Sleep 1s
Escape Sleep 700ms
Enter Sleep 3s
Screenshot wiki/shots/picker-review.png
Sleep 1s
Type "y" Sleep 5s
Screenshot wiki/shots/picker-done.png
Sleep 1s
