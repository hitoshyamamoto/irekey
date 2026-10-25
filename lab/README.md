# lab

The lab simulates devices and the services around them with containers
only, so that the whole workflow runs in CI and on a laptop without
hardware; `irekey lab up` starts it. This directory will hold the sources of
its images (simulated devices with systemd and sshd, a Modbus TCP simulator
and an IEC 60870-5-104 simulator), the fictional components, the drift
generator and the fault scenarios.

The lab is optional and separate from the irekey binary, which embeds only
the compose file and the scenario definitions and references the images by
digest. The IEC 60870-5-104 simulator is licensed under GPL-3.0: it only
ever runs in its own container image and is never linked into irekey.

Nothing is here yet; see ROADMAP.md.
