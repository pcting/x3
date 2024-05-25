xsway
==

XMonad like workspace management + dynamic named WS for sway

xsway list
-------

List all workspaces

xsway show <wsName|wsNum>
----------------------

Show or create the WS on the focused screen.
Swap workspaces if the WS is visible on another screen.

Example conf:

    set $xsway_show exec --no-startup-id xsway show

    bindsym $mod+1 $xsway_show 1
    bindsym $mod+2 $xsway_show 2
    [...]

    bindsym $mod+p exec xsway list | dmenu | xsway show

xsway rename
---------

Just rename the current workspace without changing it's binding.

Example conf:

    bindsym $mod+Shift+r exec dmenu -noinput | xsway rename

xsway bind <num>
-------------

Bind the current WS to key <num>.
If another WS is already binded to <num> bind keys are swaped.

Example conf:

    set $xsway_bind exec --no-startup-id xsway bind

    bindsym $mod+Control+1 $xsway_bind 1
    bindsym $mod+Control+2 $xsway_bind 2
    [...]

xsway swap
-------

Swap the two visible workspaces (on two screen setup).

Example conf:

    bindsym $mod+Shift+twosuperior exec xsway swap

xsway move
-------

Move the current container to workspace.
Takes care of finding the correct workspace based on name of bind key.

Example conf:

    set $xsway_move exec --no-startup-id xsway move

    bindsym $mod+Shift+1 $xsway_move 1
    bindsym $mod+Shift+2 $xsway_move 2
    [...]

    bindsym $mod+m exec xsway list | dmenu | xsway move

xsway merge
--------

Merge the current container into another non splited container.

Usage:

    xsway merge left vertical default
    xsway merge right vertical stacking
