# okapi

okapi makes the comments in a Go tree plain. It measures them. It shows the
blocks worth a rewrite. An ollama model drafts the rewrites. Someone reviews
the drafts. Then okapi applies the verdicts to the files.

By default nothing leaves the machine. If `OKAPI_OLLAMA` names an endpoint on
another machine, `draft` sends the comments there.

It is not a colour space. The okapi is the forest giraffe, the quiet one
nobody sees. It grooms itself with a long tongue, over the eyelids and into
the ears. That gets the fuss off, and that is the job.

## install

    go install github.com/janearc/okapi/cmd/okapi@latest

In this tree, `game build` writes `bin/okapi`, stamped with the commit.

Only `draft` needs more. It needs [ollama](https://ollama.com) running with a
model pulled, on this machine or wherever `OKAPI_OLLAMA` points. The default
model is `qwen3-14b-12k`.
`OKAPI_MODEL` names another model. `OKAPI_OLLAMA` names another endpoint.

## running okapi

okapi is a pzm tool. pzm is the Go library every tool here is built on. okapi
declares its verbs and pzm does the rest. It parses the arguments, writes the
help and the errors, and says what each verb does to the machine.

    okapi VERB [ARGS]

Flags take two hyphens. They may come before or after the words. Two hyphens
alone end the flags.

pzm gives okapi these:

- `okapi --help` lists the verbs.
- `okapi VERB --help` lists one verb and its flags.
- `okapi --agent-help` says the same for an agent, one labelled line each.
- `okapi --age` says which commit this build is and how long ago it was made.
- `okapi --who` says who built it.

pzm gives `--dry-run` to the verbs that write. Those are `draft` and `apply`.
With `--dry-run` they say what they would do and do none of it.

okapi exits 0 when the verb ran. It exits 1 when the verb ran and failed. It
exits 2 when nothing ran, for instance on a verb that does not exist.

The old spellings still work. Those are `okapi help`, `okapi -version` and
flags with one hyphen such as `-at`. Flags after the words are flags now.
Before, they were file names.

## what each verb does to the machine

- `measure`, `blocks`, `brief` and `version` only read.
- `draft` writes the drafts file. It asks the ollama model, on this machine
  unless `OKAPI_OLLAMA` names another one. It can hold the GPU while a local
  model answers.
- `apply` rewrites the Go files the drafts name.

`okapi --agent-help` states the same in full.

## the four moves

Measure first, so you know what you are changing:

    okapi measure mine=. stdlib=$(go env GOROOT)/src/net/http

Go files that are not tests are read. So are Markdown files. Bare paths are
named after their directory. `--at REV` reads the files at a git revision, for
a before and after.

See what would be sent, with the habit named:

    okapi blocks internal/*.go

Draft. Every block that runs past one line or has a habit goes to the model
with the voice. The drafts land beside the originals in a JSON file. Nothing
in the tree changes. Every move leaves alone a comment with `noqa` on any
line.

    okapi draft drafts.json $(git ls-files '*.go' | grep -v _test)

Check. Someone reads each draft against its code. `okapi brief drafts.json`
prints what to tell them. Their answer is one heading for each block, in this
form:

    internal/x.go:12 fix
    // the comment as it should read
    internal/x.go:40 accept
    internal/y.go:7 keep

Apply. Fix puts in the reviewer's lines. Accept puts in the draft. Keep leaves
the original. Blocks the review passed over are kept and counted as missing.

    okapi apply drafts.json review.txt

Add `FILE:LINE=keep`, `=accept` or `=fix` after the review to override one
verdict. `--repo DIR` names the tree the drafts' paths are under. The default
is the current directory.

The files may have moved under the drafts. Each comment is then found where it
now stands, as long as it stands there once. Comments that changed refuse the
whole apply before anything is written. So do comments that now stand twice.

Then read every changed line yourself before anyone else does.

## when it fails

The error names the verb first. These are the ones you will meet.

- `needs more: okapi measure NAME=PATH ...` means a word is missing. The text
  after the colon is the shape.
- `ollama is not answering at URL` means the model is not up. Start ollama or
  fix `OKAPI_OLLAMA`. Nothing was written.
- `N of M drafted and written to FILE` means the model failed part way. The
  drafts file holds the blocks done so far. Run `draft` again.
- `the comment is not what was drafted; draft again` means the file changed
  after the drafts were made. Nothing was written. Run `draft` on the current
  file.
- `the comment moved and now stands twice; draft again` means the same text is
  in two places. Nothing was written.
- `there is no verb X` and `unknown flag` mean nothing ran. The exit code is 2.
  Check the spelling against `okapi --help`.

## the voice

The voice is the rules the model is told and the examples it is shown.
`okapi draft --voice DIR` replaces the built-in voice with `DIR/rules.txt` and
`DIR/shots.txt`. The rules are prose. The shots are pairs:

    == before
    // run runs one command. if it fails, the error carries
    // the command's own output.
    func run(dir, name string, args ...string) (string, error) {
    == after
    // run execs a command in a directory; err if nonzero.

Three to five shots in your own hand do more than any rule.

## development

    GOMAXPROCS=4 go test -cover -p 2 ./...

The tests use mocks made by mockgen. Each mock has expectations and fails the
test when a call is missing or wrong. `.game` lists the mockgen commands as
`mocks` targets.

The numbers someone chose live in `internal/picked/picked-numbers.yml`. Each
has a name and says who picked it. The code asks for them by name.

## licence

Apache 2.0. The text is in the licence file beside this one.
