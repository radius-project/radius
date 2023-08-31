# Contributing Issues

You can open an issue using the form [here](https://github.com/radius-project/radius/issues/new/choose). This form will ask you to fill out a template based on the kind of issue you choose. Please fill out the form as this will help us respond to your issue. 

## Tips for creating good issues

### Use the correct template

You will save us (the maintainers) time if you want to use the right template.

- Choose 'Bug Report' if some functionality in Radius is broken, crashing, or not working as advertised.
- Choose 'Feature Request' if you have new ideas for us, or think some existing functionality should work differently.
- Choose 'Open a blank issue' (at the bottom) if neither of those is a good fit.

### Focus on the repro steps

Providing clear repro steps with code samples is the best way to get a good response to your issue. [here](https://github.com/radius-project/radius/issues?q=is%3Aissue+is%3Aclosed)'s an example.

Remember that another human will need to read your instructions and try to reproduce your steps to understand the issue.

### Tell us what you tried

If you tried to troubleshoot or workaround the problem please tell us what you tried. This will often save a lot of time in bug investigation and might help others that are working through the same issue.

### Include screenshots

You can paste screenshots directly into a Github issue!

### Include logs

The command `rad debug-logs` will connect to your cluster, and download the logs from all of Radius' components. This will output a file `debug-logs.zip` which you can attach to your issue.

Please feel free to inspect the content in `debug-logs.zip` for privacy and security before uploading. If you attach the logs to your issue then they will be public for all to see.