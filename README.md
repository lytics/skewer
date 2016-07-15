# skewer

A really hackish way of detecting significant clock skew between hosts.

If you find this useful, I feel bad for you.

## Usage

Requires agent based ssh access to all hosts.

```sh
go get github.com/lytics/skewer
skewer -hosts=host1,host2,host3,hostn
```

### Running remotely

If you're running this from a server you're ssh'd into, you need to enable
agent forwarding with either:

```sh
ssh -A remotehost
```

...or in your `~/.ssh/config`:

```
Host remotehost
    ForwardAgent yes
```

### Alerting

Use the `-alert` option to set a command to run (should be a shell script or
single binary) if clock skew is detected.

```sh
skewer -hosts=... -alert=/path/to/alert.sh
```

The `$MAXSKEW` environment variable will be set to the maximum skew detected.

## How It Works

1. Stores start time
1. Runs date on each host via ssh concurrently
1. Calculates elapsed time to run
1. If `max(date)-min(date) > elapsed` output per host times and skews
1. Sleeps `-sleep` amount and runs again

Everything is in seconds resolution, so only skews of 2 seconds or greater will
be reported.

If you want subsecond skew detection you need a more sophisticated tool.

## How It Doesn't Work

Probably a lot of ways. Consider this a toy.

There's a lot of `log.Fatalf` instead of more sophisticated error handling.
