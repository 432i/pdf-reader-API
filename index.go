package main

import (
	"bytes"
	"strings"

	"github.com/ledongthuc/pdf"

	"database/sql"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

const (
	DB_USER     = "postgres"
	DB_PASSWORD = "password"
	DB_NAME     = "pdfs"
)

type Document struct {
	Document         string `json:"document"`
	Document_id      string `json:"document_id"`
	Required_strings string `json:"required_strings"`
	Validate_strings string `json:"validate_strings"`
}

type JsonResponse struct {
	Type    string     `json:"type"`
	Data    []Document `json:"data"`
	Message string     `json:"message"`
}

type Document2 struct {
	Document         string     `json:"document"`
	Document_id      string     `json:"document_id"`
	Required_strings [][]string `json:"required_strings"`
	Validate_strings []string   `json:"validate_strings"`
}

type JsonResponse3 struct {
	Type    string      `json:"type"`
	Data    []Document3 `json:"data"`
	Message string      `json:"message"`
}

type Document3 struct {
	Document_id      string `json:"document_id"`
	Required         bool
	Required_strings []bool `json:"required_strings"`
	Validate_strings []int  `json:"validate_strings"`
	Validated        int
}

/*
Nombre: setupDB
Parámetros: -
Función: Conecta con la base de datos creada en postgres
*/
func setupDB() *sql.DB {
	url := fmt.Sprintf("postgres://%v:%v@%v:%v/%v?sslmode=disable",
		DB_USER,
		DB_PASSWORD,
		"localhost",
		"5432",
		DB_NAME)
	db, err := sql.Open("postgres", url)

	checkErr(err)
	return db
}

/*
Nombre: JaroWinklerDistance
Parámetros: dos strings que se compararan
Función: calcula y retorna la distancia de jaro-winkler, lo cual indica el grado de similitud entre los dos strings
*/
func JaroWinklerDistance(s1, s2 string) float64 {

	s1Matches := make([]bool, len(s1)) // |s1|
	s2Matches := make([]bool, len(s2)) // |s2|

	var matchingCharacters = 0.0
	var transpositions = 0.0

	// sanity checks

	// return 0 if either one is empty string
	if len(s1) == 0 || len(s2) == 0 {
		return 0 // no similarity
	}

	// return 1 if both strings are empty
	if len(s1) == 0 && len(s2) == 0 {
		return 1 // exact match
	}

	if strings.EqualFold(s1, s2) { // case insensitive
		return 1 // exact match
	}

	// Two characters from s1 and s2 respectively,
	// are considered matching only if they are the same and not farther than
	// [ max(|s1|,|s2|) / 2 ] - 1
	matchDistance := len(s1)
	if len(s2) > matchDistance {
		matchDistance = len(s2)
	}
	matchDistance = matchDistance/2 - 1

	// Each character of s1 is compared with all its matching characters in s2
	for i := range s1 {
		low := i - matchDistance
		if low < 0 {
			low = 0
		}
		high := i + matchDistance + 1
		if high > len(s2) {
			high = len(s2)
		}
		for j := low; j < high; j++ {
			if s2Matches[j] {
				continue
			}
			if s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matchingCharacters++
			break
		}
	}

	if matchingCharacters == 0 {
		return 0 // no similarity, exit early
	}

	// Count the transpositions.
	// The number of matching (but different sequence order) characters divided by 2 defines the number of transpositions
	k := 0
	for i := range s1 {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++ // increase transpositions
		}
		k++
	}
	transpositions /= 2

	weight := (matchingCharacters/float64(len(s1)) + matchingCharacters/float64(len(s2)) + (matchingCharacters-transpositions)/matchingCharacters) / 3

	//  the length of common prefix at the start of the string up to a maximum of four characters
	l := 0

	// is a constant scaling factor for how much the score is adjusted upwards for having common prefixes.
	//The standard value for this constant in Winkler's work is {\displaystyle p=0.1}p=0.1
	p := 0.1

	// make it easier for s1[l] == s2[l] comparison
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if weight > 0.7 {
		for (l < 4) && s1[l] == s2[l] {
			l++
		}

		weight = weight + float64(l)*p*(1-weight)
	}

	return weight
}

/*
Nombre: printMessage
Parámetros: string que contiene el mensaje
Función: imprime el string recibido
*/
func printMessage(message string) {
	fmt.Println("")
	fmt.Println(message)
	fmt.Println("")
}

/*
Nombre: checkErr
Parámetros: el error
Función: ejecuta el error recibido
*/
func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

/*
Nombre: readPdf
Parámetros: string con el lugar del pdf a leer
Función: retorna un string con el contenido del pdf y un error
*/
func readPdf(path string) (string, error) {
	f, r, err := pdf.Open(path)
	// remember close file
	defer f.Close()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	buf.ReadFrom(b)
	return buf.String(), nil
}

/*
Nombre:devolverPDF
Parámetros: un string con el pdf en formato base64
Función: se crea el archivo pdf desde base64 y luego se lee su contenido, retornandolo
*/
func devolverPDF(pdfb64 string) string {
	//se crea el pdf desde base64
	dec, err := b64.StdEncoding.DecodeString(pdfb64)
	checkErr(err)
	f, err := os.Create("resultado.pdf")
	checkErr(err)
	defer f.Close()

	if _, err := f.Write(dec); err != nil {
		panic(err)
	}
	if err := f.Sync(); err != nil {
		panic(err)
	}

	//leemos el pdf
	pdf.DebugOn = true
	content, err := readPdf("resultado.pdf") // Read local pdf file
	if err != nil {
		panic(err)
	}
	//fmt.Println(content)
	return content
}

/*
Nombre: checkRequiredStrings
Parámetros: el pdf en formato base64 y el arreglo de arreglos con palabras a chequear
Función: chequea si las palabras de requiredStrings están en el pdf, retornando un arreglo de booleanos
*/
func checkRequiredStrings(b64pdf string, requiredStrings [][]string) []bool {

	var booleans []bool

	content := devolverPDF(b64pdf)

	for _, s := range requiredStrings {
		flag := false
		for _, p := range s {
			//aqui estamos dentro del primer arreglo, chequear si la palabra está en el pdf
			//fmt.Println(p)
			if strings.Contains(content, p) {
				booleans = append(booleans, true)
				flag = true
				break
			}
		}
		if flag == false {
			booleans = append(booleans, false)
		}
	}

	return booleans
}

/*
Nombre: checkValidateStrings
Parámetros: el pdf en formato base64 y el arreglo de strings a chequear
Función: chequea si las palabras de arr están o si existen similares, retornando el grado de similitud
*/
func checkValidateStrings(b64pdf string, arr []string) ([]int, int) {
	var resultado []int
	var promedio int
	var max float64

	content := devolverPDF(b64pdf)
	//fmt.Println(content)

	//pasar string grande a un arreglo

	contentArray := strings.Split(content, "")
	//fmt.Println(contentArray)

	for _, p := range arr {
		//fmt.Println(p)
		max = -1.0
		for j := 0; j <= len(contentArray)-len(p); j++ {
			var temp string

			for i := 0; i < len(p); i++ { //se arma la palabra del pdf de tamaño de p (validated string)

				temp = temp + string(contentArray[i+j])

			}
			//fmt.Println(temp)

			//guardar el mayor puntaje obtenido al comparar todas las palabras

			current := JaroWinklerDistance(p, temp) * 100 //obtenemos el ptje de similitud entre ambas palabras
			//fmt.Println(current)
			if current > max {
				max = current
			}

		}

		resultado = append(resultado, int(max))
	}

	//calculamos el promedio
	sum := 0
	for i := 0; i < len(resultado); i++ {

		sum += (resultado[i])
	}

	promedio = int((float64(sum)) / (float64(len(resultado))))

	return resultado, promedio
}

//HTTP METHODS

/*
Nombre: GetDocument
Parámetros: recibe objetos de tipo http
Función: a partir del id recibido, busca el documento en la bd y lo retorna
*/
func GetDocument(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)

	documentID := params["id"]

	var response = JsonResponse{}

	if documentID == "" {
		response = JsonResponse{Type: "error", Message: "You are missing documentID parameter."}
		json.NewEncoder(w).Encode(response)
	} else {
		db := setupDB()

		printMessage("Getting document...")

		doc, err := db.Query("SELECT * FROM documents WHERE document_id = $1", documentID)

		// check errors
		checkErr(err)

		var pdf []Document

		for doc.Next() {
			var document1 string
			var document_id1 string
			var required_strings1 []byte
			var validate_strings1 []byte

			err = doc.Scan(&document1, &document_id1, &required_strings1, &validate_strings1)

			checkErr(err)

			pdf = append(pdf, Document{Document: document1, Document_id: document_id1, Required_strings: string(required_strings1), Validate_strings: string(validate_strings1)})

		}
		var response = JsonResponse{Type: "success", Data: pdf}
		json.NewEncoder(w).Encode(response)
		db.Close()
	}

}

/*
Nombre: CreateDocument
Parámetros: recibe objetos de tipo http
Función: recibe un documento, lo guarda y retorna la respuesta especificada en la prueba técnica
*/
func CreateDocument(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	var pdf Document2
	err := decoder.Decode(&pdf)
	checkErr(err)

	documentID := string(pdf.Document_id)
	document := string(pdf.Document)
	requiredStrings := (pdf.Required_strings)
	validateStrings := (pdf.Validate_strings)
	//fmt.Println(requiredStrings)
	var response = JsonResponse3{}

	if documentID == "" || document == "" || len(requiredStrings) == 0 || len(validateStrings) == 0 {
		response = JsonResponse3{Type: "error", Message: "You are missing some parameter"}
	} else {
		db := setupDB()

		printMessage("Inserting document into DB")

		//var a int
		db.QueryRow("INSERT INTO documents(pdfdocument, document_id, required_strings, validate_strings) VALUES($1, $2, $3, $4);", document, documentID, pq.Array(requiredStrings), pq.Array(validateStrings))

		// check errors
		//checkErr(err)

	}

	//akii implementar validacion ??
	requiredStringsResult := checkRequiredStrings(document, requiredStrings)
	fmt.Println(requiredStringsResult)

	validateStringsResult, promedio := checkValidateStrings(document, validateStrings)
	fmt.Println(validateStringsResult, promedio)

	//creamos el json de respuesta
	//calculamos el ponderado de required_strings
	var required bool
	flag := true
	for _, b := range requiredStringsResult {
		if b == false {
			required = false
			flag = false
			break
		}
	}
	if flag == true {
		required = true
	}

	var final []Document3
	final = append(final, Document3{Document_id: documentID, Required: required, Required_strings: requiredStringsResult, Validate_strings: validateStringsResult, Validated: promedio})
	response = JsonResponse3{Type: "success", Data: final, Message: "The pdf has been inserted successfully!"}

	json.NewEncoder(w).Encode(response)
}

func main() {

	// Init the mux router
	router := mux.NewRouter()

	// Route handles & endpoints

	// Get all movies
	router.HandleFunc("/get_document/{id}", GetDocument).Methods("GET")

	// Create a movie
	router.HandleFunc("/validate_document/", CreateDocument).Methods("POST")

	// serve the app

	fmt.Println("Server at 8000")
	log.Fatal(http.ListenAndServe(":8000", router))

}
