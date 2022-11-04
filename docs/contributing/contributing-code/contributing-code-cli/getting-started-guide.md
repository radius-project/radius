# CLI Framework

Every New command has to implement [Runner](/pkg/cli/framework/framework.go)
The Runner mimicks every radius command by having two stages: Validate and Run

### Validate

This stage should be used to accept inputs and validate which are later used to execute the command.

### Run

This stage is intended to execute, interact with RPs if needed based on the inputs in validate stage.

# New Command
### File Structure
All new commands are within [cmd](/pkg/cli/cmd/) package. The hierarchy of each command's file structure is intended to represent the syntax of the command with the word "rad" of the command representing the top level cmd directory

ex: rad resource show  would correpond to a stucture of

cmd

-- resource

-- -- show

-- -- -- show.go, show_test.go


Get started quickly by copy-pasting the template in [README](/pkg/cli/cmd//README.md)

### Testing
The integration tests of Validate stage follow a table driven format.

The integration tests of Run stage follow a nested format.

Radius cli is written in go and golang allows mocking of interfaces. As a rule of thumb, wherever cli interacts with external libraries, services and frameworks, these operations are abstracted out into interfaces to maintain a code base which is testable.