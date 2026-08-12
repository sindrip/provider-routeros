# Evidence before code

Every layer is produced by the layer before it, starting from the device:
harness boots a disposable router → probes pin evidence artifacts → the IR
compiles them → the generator emits Kinds and descriptors → the engine
reconciles what the generator emitted. Nothing is derived from documentation
— the manual has been caught wrong in both directions — and no schema is
written by hand. Probes speak REST because it returns machine-shaped JSON;
console output is presentation, not evidence (SSH stays available for
questions only the console can answer). Evidence is re-pinned on current
stable per probe run, not copied forward: an artifact the pipeline cannot
reproduce is a claim, not evidence.
