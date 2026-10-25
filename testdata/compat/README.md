# Compatibility zoo

This directory will hold artifacts written by every released version of
irekey, one directory per version (`testdata/compat/<version>/`): bundles,
device journals and deployment repository files. The tests of the current
code must still read all of them, so a change that breaks reading what an
earlier release wrote fails before it is merged.

Files are added when a version is released and kept exactly as that
version wrote them.

Nothing is here yet; see ROADMAP.md.
