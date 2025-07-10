package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Comment represents the database model for comments
type Comment struct {
	ID      uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	PMID    string `json:"pmid" gorm:"size:20;not null"`
	Aspect  string `json:"aspect" gorm:"size:10;not null"`
	Model   string `json:"model" gorm:"size:50;not null"`
	Comment string `json:"comment" gorm:"type:text;not null"`
}

// TableName overrides the table name used by GORM to match the Python app
func (Comment) TableName() string {
	return "comment"
}

// ProcessRequest represents the structure for file processing
type ProcessRequest struct {
	Reference []map[string]interface{} `json:"reference"`
	Responses []map[string]interface{} `json:"responses"`
}

var db *gorm.DB

func init() {
	var err error

	// Create instance directory if it doesn't exist
	err = os.MkdirAll("instance", 0755)
	if err != nil {
		log.Fatal("Failed to create instance directory:", err)
	}

	db, err = gorm.Open(sqlite.Open("instance/comments.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate the schema - this will use the TableName() method to create "comment" table
	db.AutoMigrate(&Comment{})
}

func main() {
	r := gin.Default()

	// Load HTML templates
	r.LoadHTMLGlob("templates/*")

	// Serve static files
	r.Static("/static", "./static")

	// Routes
	r.GET("/", indexHandler)
	r.GET("/api/comments/by-model", getCommentsByModel)
	r.POST("/process", processFiles)
	r.POST("/comments", addComment)
	r.GET("/comments/:pmid", getComments)
	r.DELETE("/comments/:id", deleteComment)

	fmt.Println("Server starting on port 8080...")
	r.Run(":8080")
}

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", nil)
}

func getCommentsByModel(c *gin.Context) {
	log.Printf("=== Getting all comments grouped by model ===")

	// Use raw SQL to ensure we get all fields correctly
	rows, err := db.Raw("SELECT id, pmid, aspect, model, comment FROM comment").Rows()
	if err != nil {
		log.Printf("Error fetching all comments: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.PMID, &comment.Aspect, &comment.Model, &comment.Comment); err != nil {
			log.Printf("Error scanning comment: %v", err)
			continue
		}
		comments = append(comments, comment)
	}

	log.Printf("Found %d total comments", len(comments))

	// Group comments by model
	commentsByModel := make(map[string][]Comment)
	for _, comment := range comments {
		model := comment.Model
		commentsByModel[model] = append(commentsByModel[model], comment)
	}

	log.Printf("Comments grouped by %d models", len(commentsByModel))
	for model, modelComments := range commentsByModel {
		log.Printf("Model '%s': %d comments", model, len(modelComments))
	}

	// Sort comments within each model by pmid and aspect
	for model := range commentsByModel {
		sort.Slice(commentsByModel[model], func(i, j int) bool {
			if commentsByModel[model][i].PMID == commentsByModel[model][j].PMID {
				return commentsByModel[model][i].Aspect < commentsByModel[model][j].Aspect
			}
			return commentsByModel[model][i].PMID < commentsByModel[model][j].PMID
		})
	}

	// Ensure all PMIDs are properly formatted in response
	cleanedCommentsByModel := make(map[string][]interface{})
	for model, modelComments := range commentsByModel {
		cleanedModelComments := make([]interface{}, len(modelComments))
		for i, comment := range modelComments {
			cleanedModelComments[i] = map[string]interface{}{
				"id":         comment.ID,
				"pmid":       pmidToString(comment.PMID),
				"aspect":     comment.Aspect,
				"model":      comment.Model,
				"comment":    comment.Comment,
				"syncStatus": "synced", // All comments from DB are already synced
			}
		}
		cleanedCommentsByModel[model] = cleanedModelComments
	}

	c.JSON(http.StatusOK, cleanedCommentsByModel)
}

func processFiles(c *gin.Context) {
	log.Println("=== Processing files ===")
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse multipart form"})
		return
	}

	referenceFiles := form.File["reference"]
	responseFiles := form.File["responses"]

	if len(referenceFiles) == 0 || len(responseFiles) == 0 {
		log.Printf("Missing files: reference=%d, responses=%d", len(referenceFiles), len(responseFiles))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing files"})
		return
	}

	log.Printf("Processing %d reference files and %d response files", len(referenceFiles), len(responseFiles))

	// Process reference file
	referenceData := []map[string]interface{}{}
	pmidAspects := make(map[string][]string)
	pmidOrder := []string{}
	aspectOrder := []string{}

	referenceFile := referenceFiles[0]
	if err := processJSONLFile(referenceFile, &referenceData, pmidAspects, &pmidOrder, &aspectOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process reference file"})
		return
	}

	// Process response files
	responseData := make(map[string][]map[string]interface{})
	for _, responseFile := range responseFiles {
		modelName := strings.TrimSuffix(responseFile.Filename, filepath.Ext(responseFile.Filename))
		responses := []map[string]interface{}{}
		if err := processJSONLFile(responseFile, &responses, pmidAspects, &pmidOrder, &aspectOrder); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process response file"})
			return
		}
		responseData[modelName] = responses
	}

	// Organize data by PMID
	organizedData := map[string]interface{}{
		"pmid_order": pmidOrder,
	}

	for _, pmid := range pmidOrder {
		// Sort PMID's aspects according to global aspect order
		pmidAspectList := make([]string, len(pmidAspects[pmid]))
		copy(pmidAspectList, pmidAspects[pmid])
		sort.Slice(pmidAspectList, func(i, j int) bool {
			return indexOf(aspectOrder, pmidAspectList[i]) < indexOf(aspectOrder, pmidAspectList[j])
		})

		reference := make(map[string][]map[string]interface{})
		for _, aspect := range pmidAspectList {
			reference[aspect] = []map[string]interface{}{}
		}

		organizedData[pmid] = map[string]interface{}{
			"reference": reference,
			"responses": make(map[string]interface{}),
			"aspects":   pmidAspectList,
		}
	}

	// Add reference data
	for _, ref := range referenceData {
		if pmidVal, ok := ref["pmid"]; ok {
			pmid := pmidToString(pmidVal)
			ref["pmid"] = pmid // Store the properly formatted PMID back
			if orgData, exists := organizedData[pmid]; exists {
				if aspect, ok := ref["aspect"]; ok {
					aspectStr := fmt.Sprintf("%v", aspect)
					orgDataMap := orgData.(map[string]interface{})
					referenceMap := orgDataMap["reference"].(map[string][]map[string]interface{})
					if _, aspectExists := referenceMap[aspectStr]; aspectExists {
						referenceMap[aspectStr] = append(referenceMap[aspectStr], ref)
					}
				}
			}
		}
	}

	// Add response data
	for model, responses := range responseData {
		for _, resp := range responses {
			if pmidVal, ok := resp["pmid"]; ok {
				pmid := pmidToString(pmidVal)
				resp["pmid"] = pmid // Store the properly formatted PMID back
				if orgData, exists := organizedData[pmid]; exists {
					orgDataMap := orgData.(map[string]interface{})
					responsesMap := orgDataMap["responses"].(map[string]interface{})

					if _, modelExists := responsesMap[model]; !modelExists {
						modelResponses := make(map[string][]map[string]interface{})
						aspects := orgDataMap["aspects"].([]string)
						for _, aspect := range aspects {
							modelResponses[aspect] = []map[string]interface{}{}
						}
						responsesMap[model] = modelResponses
					}

					if aspect, ok := resp["aspect"]; ok {
						aspectStr := fmt.Sprintf("%v", aspect)
						modelResponsesMap := responsesMap[model].(map[string][]map[string]interface{})
						if _, aspectExists := modelResponsesMap[aspectStr]; aspectExists {
							modelResponsesMap[aspectStr] = append(modelResponsesMap[aspectStr], resp)
						}
					}
				}
			}
		}
	}

	log.Printf("=== Final organized data ===")
	log.Printf("PMID order: %v", organizedData["pmid_order"])

	// Recursively ensure all PMID values are properly formatted strings
	cleanedData := ensureAllPMIDsAreStrings(organizedData).(map[string]interface{})

	// Log a sample PMID entry to debug
	finalPMIDOrder := cleanedData["pmid_order"].([]string)
	if len(finalPMIDOrder) > 0 {
		samplePMID := finalPMIDOrder[0]
		log.Printf("Sample PMID '%s' data: %+v", samplePMID, cleanedData[samplePMID])
	}

	c.JSON(http.StatusOK, cleanedData)
}

func processJSONLFile(file *multipart.FileHeader, data *[]map[string]interface{}, pmidAspects map[string][]string, pmidOrder *[]string, aspectOrder *[]string) error {
	f, err := file.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		var record map[string]interface{}

		// Use a decoder that preserves numbers to avoid float64 conversion
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.UseNumber()
		if err := decoder.Decode(&record); err != nil {
			log.Printf("Error parsing JSON line: %v, line: %s", err, line)
			continue
		}

		// Fix PMID formatting immediately after parsing
		if pmidVal, ok := record["pmid"]; ok {
			pmid := pmidToString(pmidVal)
			record["pmid"] = pmid // Store the properly formatted PMID back

			if !contains(*pmidOrder, pmid) {
				*pmidOrder = append(*pmidOrder, pmid)
			}

			// Initialize aspects list for this PMID if not exists
			if _, exists := pmidAspects[pmid]; !exists {
				pmidAspects[pmid] = []string{}
			}

			// Track aspect for this PMID while maintaining order
			if aspectVal, ok := record["aspect"]; ok {
				aspect := fmt.Sprintf("%v", aspectVal)
				// Add to global aspect order if not there
				if !contains(*aspectOrder, aspect) {
					*aspectOrder = append(*aspectOrder, aspect)
				}
				// Add to PMID-specific aspects if not there
				if !contains(pmidAspects[pmid], aspect) {
					pmidAspects[pmid] = append(pmidAspects[pmid], aspect)
				}
			}
		}

		*data = append(*data, record)
	}

	return scanner.Err()
}

func addComment(c *gin.Context) {
	log.Printf("=== Adding comment ===")
	var comment Comment
	if err := c.ShouldBindJSON(&comment); err != nil {
		log.Printf("Error binding JSON for comment: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Adding comment: PMID=%s, Aspect=%s, Model=%s, Comment=%s",
		comment.PMID, comment.Aspect, comment.Model, comment.Comment)

	// Use raw SQL to insert comment to ensure compatibility with existing table
	result := db.Exec("INSERT INTO comment (pmid, aspect, model, comment) VALUES (?, ?, ?, ?)",
		comment.PMID, comment.Aspect, comment.Model, comment.Comment)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	// Get the last inserted row ID from GORM
	var lastID int64
	db.Raw("SELECT last_insert_rowid()").Scan(&lastID)
	comment.ID = uint(lastID)

	log.Printf("Comment created successfully with ID: %d", comment.ID)

	// Ensure PMID is properly formatted in response and include sync status
	cleanedComment := map[string]interface{}{
		"id":         comment.ID,
		"pmid":       pmidToString(comment.PMID),
		"aspect":     comment.Aspect,
		"model":      comment.Model,
		"comment":    comment.Comment,
		"syncStatus": "synced", // Mark as synced since it was successfully saved to server
	}

	c.JSON(http.StatusOK, cleanedComment)
}

func getComments(c *gin.Context) {
	pmid := c.Param("pmid")
	log.Printf("=== Getting comments for PMID: %s ===", pmid)

	// Use raw SQL to ensure we get all fields correctly
	rows, err := db.Raw("SELECT id, pmid, aspect, model, comment FROM comment WHERE pmid = ?", pmid).Rows()
	if err != nil {
		log.Printf("Error fetching comments for PMID %s: %v", pmid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.PMID, &comment.Aspect, &comment.Model, &comment.Comment); err != nil {
			log.Printf("Error scanning comment: %v", err)
			continue
		}
		comments = append(comments, comment)
	}

	log.Printf("Found %d comments for PMID %s", len(comments), pmid)

	// Ensure PMIDs are properly formatted in response
	cleanedComments := make([]interface{}, len(comments))
	for i, comment := range comments {
		cleanedComments[i] = map[string]interface{}{
			"id":         comment.ID,
			"pmid":       pmidToString(comment.PMID),
			"aspect":     comment.Aspect,
			"model":      comment.Model,
			"comment":    comment.Comment,
			"syncStatus": "synced", // All comments from DB are already synced
		}
	}

	c.JSON(http.StatusOK, cleanedComments)
}

func deleteComment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	// Use raw SQL to find the comment
	var comment Comment
	row := db.Raw("SELECT id, pmid, aspect, model, comment FROM comment WHERE id = ?", uint(id)).Row()
	if err := row.Scan(&comment.ID, &comment.PMID, &comment.Aspect, &comment.Model, &comment.Comment); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}

	// Use raw SQL to delete the comment
	if err := db.Exec("DELETE FROM comment WHERE id = ?", uint(id)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

// ensureAllPMIDsAreStrings recursively walks through the data structure and
// ensures all PMID values are properly formatted strings
func ensureAllPMIDsAreStrings(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			if key == "pmid" {
				// Convert PMID to proper string format
				result[key] = pmidToString(value)
				log.Printf("Cleaned PMID: %v -> %s", value, result[key])
			} else {
				// Recursively process other values
				result[key] = ensureAllPMIDsAreStrings(value)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = ensureAllPMIDsAreStrings(item)
		}
		return result
	case []map[string]interface{}:
		result := make([]map[string]interface{}, len(v))
		for i, item := range v {
			result[i] = ensureAllPMIDsAreStrings(item).(map[string]interface{})
		}
		return result
	default:
		// Return as-is for primitive types
		return v
	}
}

// pmidToString converts a PMID value to string without scientific notation
func pmidToString(pmidVal interface{}) string {
	switch v := pmidVal.(type) {
	case string:
		return v
	case float64:
		// Use %.0f to avoid scientific notation for large numbers
		result := fmt.Sprintf("%.0f", v)
		log.Printf("PMID conversion: float64 %v -> %s", v, result)
		return result
	case float32:
		result := fmt.Sprintf("%.0f", float64(v))
		log.Printf("PMID conversion: float32 %v -> %s", v, result)
		return result
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case json.Number:
		// Handle json.Number specifically to avoid scientific notation
		// First try to parse as int64, then fall back to string
		if intVal, err := v.Int64(); err == nil {
			result := fmt.Sprintf("%d", intVal)
			log.Printf("PMID conversion: json.Number (int) %v -> %s", v, result)
			return result
		}
		// If it's not an integer, convert directly to string
		result := string(v)
		log.Printf("PMID conversion: json.Number (string) %v -> %s", v, result)
		return result
	default:
		// Fallback to string conversion
		result := fmt.Sprintf("%v", v)
		log.Printf("PMID conversion: unknown type %T %v -> %s", v, v, result)
		return result
	}
}
