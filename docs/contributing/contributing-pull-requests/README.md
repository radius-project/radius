# Contributing Pull Requests

## What to work on

We welcome small pull request contributions from anyone (docs improvements, bug fixes, minor features.) as long as they follow a few guidelines:

- For very minor changes like correcting a typo feel free to just send a pull request without any ceremony. Otherwise ... 
- Please start by [choosing an existing issue](https://github.com/project-radius/radius/issues), or [opening an issue](https://github.com/project-radius/radius/issues/new/choose) to work on.
- The maintainers will respond to your issue, please work with the maintainers to ensure that what you're doing is in scope for the project before writing any code.
- If you have any doubt whether a contribution would be valuable, feel free to ask.

We the maintainers have discretion over what features and pull requests we accept. Please understand that we are responsible for the long-term support and maintenance of Radius, and so we sometimes need to make hard decisions to limit the scope. For another perspective on this, we really like this [article](https://www.igvita.com/2011/12/19/dont-push-your-pull-requests/).

## Sending a pull request

> 💡 If you're new to Go or new to open source, our [first commit guide](./../contributing-code/contributing-code-first-commit/) can walk you through the process of making code changes and submitting a pull request.

Please open pull requests against the `main` branch (the default) unless otherwise instructed.

When opening a pull request, the form will be pre-populated with our template. Please fill out the template to provide structure to your PR message. If you've already written a good commit message (see below) it will be easy to use with our template.

A pull request will need to pass the following checkpoints to be accepted:

- Initial review: a maintainer will review your summary and make sure an appropriate issue is linked
- Testing: automated tests will run against your changes
- Code review: you will get feedback from a maintainer or other contributors in the form of comments

We expect that contributors have run basic validations (`make build test lint`) before sending a pull request. See [building the repo](../contributing-code/contributing-code-building/) for more information. If you get stuck during this step feel free to open the pull request anyway and ask for help in our [forum](https://discordapp.com/channels/1113519723347456110/1115302284356767814).

## Filling out the pull request template

Our pull request template will ask you to choose one of three options from a list when submitting the pull request. Telling us what type of change the pull request contains will help us author the release notes and informs reviewers about what to look for in your pull request.

The following tips should help you decide which option to choose:

- Does this change fix a bug in Radius? We define a bug as a case where Radius does not work as advertised, or where a crash or some other kind of internal failure occurs. If you said Yes, then choose the first option (Bugfix).
- Does this change introduce new features or behaviors? Does this change modify an existing feature of Radius in a way that's visible to users? If you said Yes, choose the second option (Feature).
- If neither of the two previous options sounds right, and there's no user-visible change in your pull request then choose the third option (Task).

We use Task as a catch-all for changes that have no direct user-visible impact. In practice many changes are Tasks, and we don't include them in the release notes. This includes minor refactors, code/style cleanup, test improvements, correcting misspellings in comments, changes to build processes, etc.


### Copilot summary

Our pull-request template includes a summary from Github Copilot. This will generate a description of your changes as well as links to the most relevant files. 

You can find an example [here](https://github.com/project-radius/radius/pull/5614).

## Tips

Keep reading for some tips about how to get your pull requests accepted!

## Writing a good commit message

We value good commit messages that are descriptive and meaningful at a glance. A good format to follow is like the following:

```txt
<short description>

Fixes: #<issue>

<a longer description that includes>

- a summary of the changes being made
- the rationale for the change
- (optional) anything tricky or difficult as a heads up for reviewers
- (optional) additional follow up work that should be done (with links)
```

We **squash** pull-requests as part of the merge process, which means that intermediate commits will have their messages appended. We prefer to have a single commit in the git history for each PR.

## Automated tests

Our GitHub Actions workflows will run against your pull request to validate the changes. This will run the unit tests, integration tests, and functional tests.

Ideally everything works the first time, but you may not be so lucky! Our automation will add comments to your pull request that helps explain what's happening. This will include links to logs where you can diagnose what's happening.

If you get stuck with a failure you can't understand, feel free to ask the maintainers for help.

### CodeQL security analysis

We run [CodeQL](https://codeql.github.com/) as part of the pull-request process for security analysis. If the CodeQL analysis finds a security issue it will be reported as part of the PR checks. CodeQL is not currently required to pass for a PR to be merged, as it may be triggered by other alerts within the repo.

If CodeQL fails due to your changes, please work with the maintainers to resolve the issue.


## Code review

The maintainers or other contributors will add comments to your pull request giving feedback, asking questions, and making suggestions. Please respond to these comments to either continue the discussion or explain whether or not you plan to address the feedback. Ultimately, accepting a pull request is at the maintainer's discretion.

### Being proactive 

It can be helpful for you to comment on your own PR to point out relevant locations, decisions, opportunities for feedback, and tricky parts. This will help reviewers focus their attention as well as save them time.

### Resolving Feedback

You can "resolve" comments on your pull request when you've addressed the feedback: either through discussion or through making a code change. As the contributor of the pull-request feel free to mark comments as resolved when you feel like you've done a reasonable job addressing the feedback.

If you are the code reviewer, it's your responsibility to follow up (politely) if you feel your feedback has not been addressed adequately.

### Participating in code review

We welcome any contributor or community member to engage with any pull request on our repository. Feel free to make suggestions for improvements and ask questions that are relevant. If you're asking questions for your learning, please make it clear that your questions are "non-blocking" for the pull request.