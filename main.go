package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"io"

	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
)

var authToken string

func main() {
	godotenv.Load()
	authToken = os.Getenv("AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("AUTH_TOKEN environment variable not set")
	}

	http.HandleFunc("/", formHandler)
	http.HandleFunc("/upload", uploadHandler)

	fmt.Println("Server is listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func formHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `<html>
	<body>
		<form action="/upload" method="post" enctype="multipart/form-data">
			<input type="hidden" name="auth" value="{{.AuthToken}}">
			<input type="file" name="data">
			<input type="submit" value="Upload">
		</form>
	</body>
	</html>`

	t, err := template.New("form").Parse(tmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		AuthToken string
	}{
		AuthToken: authToken,
	}

	if err := t.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	auth := r.FormValue("auth")
	if auth != authToken {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	file, header, err := r.FormFile("data")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Check if the content type is an image
	if header.Header.Get("Content-Type")[:5] != "image" {
		http.Error(w, "Not an image", http.StatusForbidden)
		return
	}

	// Check file size
	fileSize := header.Size
	if fileSize > 8<<20 {
		http.Error(w, "File size exceeds 8MB", http.StatusForbidden)
		return
	}

	// Save the file to a temporary location
	tempFile, err := os.CreateTemp("", "upload-*.png")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write image metadata to PostgreSQL database
	db, err := sql.Open("postgres", "postgres://postgres:secret@localhost/postgres?sslmode=disable")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS images (id SERIAL PRIMARY KEY, filename TEXT, content_type TEXT, size INT)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("INSERT INTO images (filename, content_type, size) VALUES ($1, $2, $3)", filepath.Base(tempFile.Name()), header.Header.Get("Content-Type"), fileSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "File uploaded successfully")
}