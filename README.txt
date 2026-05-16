The user wants to know the exact commands and directories to start both servers from command prompt.

Backend — from c:\Users\kevin\CascadeProjects\lexicon\backend:

go run ./cmd/server
Frontend — from c:\Users\kevin\CascadeProjects\lexicon\frontend:

npx vite --host
Then open http://localhost:5173.

If you get a port-already-in-use error, kill the old process first: netstat -ano | findstr :8787 (or :5173), note the PID, then taskkill /PID <pid> /F.