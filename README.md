# Tetris
A tetris clone modeled mostly after the NES version, but with some updates such
as ghost pieces and hard dropping. Does not use the console input driver.

If you do not want a gitlab account but have problems or suggestions,
send an email to my gmail address: bendypauldron

### Features
* Less than 500 lines of code.
* Hard drop
* Ghost piece
* Standard Rotation System.
* Line clear delay, lock delay, fps, soft drop rate, and DAS delay & rate (configurable as consts in source for now).
* Next piece preview.
* Hold piece. Hold piece buffering.
* Classic scoring.
* NESish tetris ui layout.

### Controls
Controls use scancodes. You can find a list of codes in linux/include/uapi/linux/input-event-codes.h
and change these in the source.

w - hold, a - left, s - soft drop, d - right, space - hard drop, j - rotate left, k - rotate right

### Install (or update)
```shell
go get -u gitlab.com/meutraa/tetris
```

##### Cross Compiling
See https://golang.org/doc/install/source#environment for GOOS and GOARCH combinations.
```shell
git clone git@gitlab.com:meutraa/tetris.git
cd tetris
GOOS=linux GOARCH=arm go build
```

### Usage
User must be a member of the `input` group.

```shell
root $ gpasswd -a "$USER" input
root $ reboot
```

```shell
tetris -i /dev/input/by-id/kbd-your-keyboard-name
```
