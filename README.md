# calendarmigrator
A go app to help migrate events from one Google Calendar to another

## Setup

To use this, you will need to follow
[the instructions to set up OAuth for Google Calendar](https://developers.google.com/calendar/api/quickstart/go#set_up_your_environment).
When configuring the oauth consent screen, enter `http://localhost:42069`
for the application home page.

When you are finished, you should have a `credentials.json` file living in
the top level directory of this project.

## Running

```bash
go run migrator.go
```

The program will print two oauth links. For the first, authorize the account
you want to move events *from*. For the second, authorize the account you want
to move events *to*.

In case of failure to copy, the original event will not be deleted.

At the end of running, the events that failed to copy or failed to be deleted
will have their dates printed so that you can manually migrate them.
