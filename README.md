# leaf-homekit

HomeKit support for the Nissan Leaf using
[hc](https://github.com/brutella/hc) and my [Leaf Go
library](https://github.com/joeshaw/leaf).

When running, this service publishes a single HomeKit accessory
exposing three services:

1. A battery service indicating the current charge of your Leaf and
   its charging status.
1. A switch service indicating whether the Leaf is currently charging.
   If the Leaf is plugged in but not charging, you can flip this
   switch on to begin charging the vehicle.
1. A switch service for the Leaf's climate control.  Flipping this
   switch on starts the vehicle's climate control system.

The Nissan API does not expose climate control status, and it is not
possible to disable charging once it has started.  As a result, the two
switches are stateless and will always reset to the off position.
HomeKit does not expose a stateless switch (button) service we can use.

After the vehicle is paired with your iOS Home app, you can control it
with any service that integrates with HomeKit, including Siri ("How
much battery does the Leaf have?") and Apple Watch.  If you have a
home hub like an Apple TV or iPad, you can control the Leaf remotely.

## Installing

The tool can be installed with:

    go get -u github.com/joeshaw/leaf-homekit

You will need a configuration file with your Nissan username and
password.  The format is the same as the config file for the
[leaf](https://github.com/joeshaw/leaf), so you can use it for both
tools.

You will need to create a file like `~/.leaf`:

```
username foo@example.com
password carwingsPassw0rd
```

Then you can run the service:

    leaf-homekit -config ~/.leaf

The service will make an initial call to the Nissan service to get the
current battery information -- this can take nearly 30 seconds -- before
it exposes the accessory to HomeKit.

To pair, open up your Home iOS app, click the + icon, choose "Add
Accessory" and then tap "Don't have a Code or Can't Scan?"  You should
see the Leaf under "Nearby Accessories."  Tap that and enter the PIN
00102003 (or whatever you chose in your config file).

## Contributing

Issues and pull requests are welcome.  When filing a PR, please make
sure the code has been run through `gofmt`.

## License

Copyright 2020 Joe Shaw

`leaf-homekit` is licensed under the MIT License.  See the LICENSE
file for details.


