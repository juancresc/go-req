```
go run main.go --rps 1 --address https://www.somesite.com --headers x-canary:true

go run main.go --duration 5s --rps 1 --address https://www.somesite.com --headers x-canary:true --authentication xxx
```

## to dos

- [ ] Avg of requests duration
- [ ] log with errors that does not dissapear