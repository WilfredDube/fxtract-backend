package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/WilfredDube/fxtract-backend/entity"
	"github.com/WilfredDube/fxtract-backend/helper"
	"github.com/WilfredDube/fxtract-backend/service"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type controller struct {
	userService           service.UserService
	jwtService            service.JWTService
	cadFileService        service.CadFileService
	projectService        service.ProjectService
	processingPlanService service.ProcessingPlanService
}

// ProjectController -
type ProjectController interface {
	AddProject(w http.ResponseWriter, r *http.Request)
	Upload(w http.ResponseWriter, r *http.Request)
	uploadHandler(r *http.Request, projectFolder string, id primitive.ObjectID) (*[]entity.CADFile, error)
	FindByID(w http.ResponseWriter, r *http.Request)
	FindCADFileByID(w http.ResponseWriter, r *http.Request)
	FindAll(w http.ResponseWriter, r *http.Request)
	FindAllCADFiles(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	DeleteCADFile(w http.ResponseWriter, r *http.Request)
}

// NewProjectController -
func NewProjectController(service service.ProjectService, uService service.UserService, cService service.CadFileService, pPlanService service.ProcessingPlanService, jwtService service.JWTService) ProjectController {
	return &controller{
		userService:           uService,
		cadFileService:        cService,
		projectService:        service,
		processingPlanService: pPlanService,
		jwtService:            jwtService,
	}
}

// NewProject - add a new project
func (c *controller) AddProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	var response *entity.Project

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		id := claims["user_id"].(string)
		OwnerID, _ := primitive.ObjectIDFromHex(id)

		if _, err := c.userService.Profile(id); err != nil {
			response := helper.BuildErrorResponse("Invalid token", "User does not exist", helper.EmptyObj{})
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		project := &entity.Project{}
		err := json.NewDecoder(r.Body).Decode(project)
		if err != nil {
			response := helper.BuildErrorResponse("Failed to process request", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		result := c.projectService.IsDuplicate(project.Title, OwnerID)
		if result == true {
			response := helper.BuildErrorResponse("Project already exist", "Duplicate request", helper.EmptyObj{})
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		err = c.projectService.Validate(project)
		if err != nil {
			response := helper.BuildErrorResponse("Failed to process request", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		project.ID = primitive.NewObjectID()
		project.OwnerID = OwnerID
		project.CreatedAt = time.Now().Unix()

		response, err = c.projectService.Create(project)
		if err != nil {
			response := helper.BuildErrorResponse("Failed to process request", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		projectFolder := response.OwnerID.Hex() + "/" + response.ID.Hex()
		helper.CreateFolder(projectFolder, false)

		// res := helper.BuildResponse(true, "OK", response)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
	}

	res := helper.BuildErrorResponse("Failed to process request", "Project creation failed", helper.EmptyObj{})
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(res)
}

// FindByID -
func (c *controller) FindByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		id := claims["user_id"].(string)

		if _, err := c.userService.Profile(id); err != nil {
			response := helper.BuildErrorResponse("Invalid token", "User does not exist", helper.EmptyObj{})
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		params := mux.Vars(r)
		id = params["id"]

		project, err := c.projectService.Find(id)
		if err != nil {
			res := helper.BuildErrorResponse("Project not found", "Unknown project id", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		// res := helper.BuildResponse(true, "OK!", project)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(project)
		return
	}

	res := helper.BuildErrorResponse("Project not found", "Unknown project id", helper.EmptyObj{})
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(res)
}

// FindAll -
func (c *controller) FindAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		ownerID := claims["user_id"].(string)

		projects, err := c.projectService.FindAll(ownerID)
		if err != nil {
			res := helper.BuildErrorResponse("Project not found", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		// res := helper.BuildResponse(true, "OK", projects)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(projects)
		return
	}

	res := helper.BuildErrorResponse("Project not found", "Unknown project id", helper.EmptyObj{})
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(res)
}

// Delete -
func (c *controller) Delete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		id := claims["user_id"].(string)

		if _, err := c.userService.Profile(id); err != nil {
			response := helper.BuildErrorResponse("Invalid token", "User does not exist", helper.EmptyObj{})
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		params := mux.Vars(r)
		id = params["id"]

		project, err := c.projectService.Find(id)
		if err != nil {
			response := helper.BuildErrorResponse("Failed to process request", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		deleteCount, err := c.projectService.Delete(id)
		if err != nil {
			response := helper.BuildErrorResponse("Failed to process request", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		if deleteCount == 0 {
			response := helper.BuildErrorResponse("Failed to process request", "Project not found", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(response)
			return
		}

		// TODO: delete project folder
		projectFolder := project.OwnerID.Hex() + "/" + project.ID.Hex()
		helper.DeleteFolder(projectFolder)

		c.cadFileService.CascadeDelete(project.ID.Hex())

		res := helper.BuildResponse(true, "OK", helper.EmptyObj{})
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		return
	}

	response := helper.BuildErrorResponse("Failed to process request", "Project deletion failed", helper.EmptyObj{})
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(response)
}

// Upload - add a new cadFile
func (c *controller) Upload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		ownerID, _ := primitive.ObjectIDFromHex(claims["user_id"].(string))

		params := mux.Vars(r)
		id, err := primitive.ObjectIDFromHex(params["id"])

		project, err := c.projectService.Find(params["id"])
		if err != nil {
			res := helper.BuildErrorResponse("Project error", "The project you want to upload to does not exist", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		if project.OwnerID != ownerID {
			res := helper.BuildErrorResponse("Project owner does not exist", "Token error", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}
		sharedFolder := "/home/unbusy/Projects/fxtract/fxtract-api/uploads/"

		projectFolder := sharedFolder + project.OwnerID.Hex() + "/" + project.ID.Hex()

		uploadedFiles, err := c.uploadHandler(r, projectFolder, id)
		if err != nil {
			res := helper.BuildErrorResponse("Upload error", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(res)
			return
		}

		res := helper.BuildResponse(true, "Upload complete : OK!", uploadedFiles)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		return
	}

	res := helper.BuildErrorResponse("Upload error", "File upload failed", helper.EmptyObj{})
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(res)
	return
}

func (c *controller) uploadHandler(r *http.Request, projectFolder string, id primitive.ObjectID) (*[]entity.CADFile, error) {
	var uploadedFiles []entity.CADFile
	tempCache := make(map[string]string)
	fileCache := make(map[string]entity.CADFile)

	// 32 MB is the default used by FormFile()
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}

	// Get a reference to the fileHeaders.
	// They are accessible only after ParseMultipartForm is called
	files := r.MultipartForm.File["file"]
	material := r.MultipartForm.Value["material"][0]

	nFiles := len(files)

	if nFiles == 0 {
		return nil, fmt.Errorf("Select a file(s) to upload")
	}

	if (nFiles % 2) != 0 {
		return nil, fmt.Errorf("Each STEP file must be uploaded with its corresponding obj file")
	}

	if !helper.UploadBalanced(files) {
		return nil, fmt.Errorf("Unbalanced: Each STEP file must be uploaded with its corresponding obj file")
	}

	for _, fileHeader := range files {
		// Restrict the size of each uploaded file to 1MB.
		// To prevent the aggregate size from exceeding
		// a specified value, use the http.MaxBytesReader() method
		// before calling ParseMultipartForm()
		if fileHeader.Size > MaxUploadSize {
			return nil, fmt.Errorf("The uploaded image is too big: %s. Please use an image less than 1MB in size", fileHeader.Filename)
		}

		ext := filepath.Ext(fileHeader.Filename)
		if ext != ".stp" && ext != ".step" && ext != ".obj" {
			return nil, fmt.Errorf("The provided file format is not allowed. %s", ext)
		}

		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			return nil, err
		}

		defer file.Close()

		buff := make([]byte, 512)
		_, err = file.Read(buff)
		if err != nil {
			return nil, err
		}

		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		processed := false
		newName := time.Now().UnixNano()
		filename := helper.FileNameWithoutExtSlice(fileHeader.Filename)
		if _, ok := tempCache[filename]; ok {
			newName, _ = strconv.ParseInt(tempCache[filename], 10, 64)
			processed = true
		}

		f, err := os.Create(fmt.Sprintf(projectFolder+"/%d%s", newName, filepath.Ext(fileHeader.Filename)))
		if err != nil {
			return nil, err
		}

		defer f.Close()

		_, err = io.Copy(f, file)
		if err != nil {
			return nil, err
		}

		// insert cad file file metadata into database
		var cadFile entity.CADFile

		if processed == false {
			tempCache[filename] = helper.FileNameWithoutExtSlice(filepath.Base(f.Name()))

			cadFile.ID = primitive.NewObjectID()
			cadFile.FileName = filename + ".stp"
			if ext == ".stp" || ext == ".step" {
				cadFile.StepURL = f.Name()
			} else {
				cadFile.ObjpURL = f.Name()
			}

			cadFile.Material = material
			cadFile.Filesize = fileHeader.Size
			cadFile.CreatedAt = time.Now().Unix()
			cadFile.ProjectID = id

			fileCache[tempCache[filename]] = cadFile

			_, err := c.cadFileService.Create(&cadFile)
			if err != nil {
				return nil, err // error updloading file failed
			}
		} else {
			fl := tempCache[filename]
			cadFile = fileCache[fl]

			if ext == ".stp" || ext == ".step" {
				cadFile.StepURL = f.Name()
			} else {
				cadFile.ObjpURL = f.Name()
			}

			_, err := c.cadFileService.Update(cadFile)
			if err != nil {
				return nil, err
			}

			uploadedFiles = append(uploadedFiles, cadFile)
			delete(tempCache, filename)
		}
	}

	return &uploadedFiles, nil
}

// FindByID -
func (c *controller) FindCADFileByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {

		params := mux.Vars(r)
		id := params["id"]

		cadFile, err := c.cadFileService.Find(id)
		if err != nil {
			res := helper.BuildErrorResponse("Process failed", "CAD file not found", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		res := helper.BuildResponse(true, "OK!", cadFile)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		return
	}
}

// FindAll -
func (c *controller) FindAllCADFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		ownerID, _ := primitive.ObjectIDFromHex(claims["user_id"].(string))

		params := mux.Vars(r)
		projectID := params["id"]

		project, err := c.projectService.Find(projectID)
		if err != nil {
			res := helper.BuildErrorResponse("Project error", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		if project.OwnerID != ownerID {
			res := helper.BuildErrorResponse("Project owner does not exist", "Token error", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		cadFiles, err := c.cadFileService.FindAll(projectID)
		if err != nil {
			res := helper.BuildErrorResponse("Process failed", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		res := helper.BuildResponse(true, "OK!", cadFiles)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		return
	}
}

// Delete -
func (c *controller) DeleteCADFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token, err := c.jwtService.GetAuthenticationToken(r, "fxtract")
	if err != nil {
		response := helper.BuildErrorResponse("Unauthorised", "User not authenticated", helper.EmptyObj{})
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		params := mux.Vars(r)
		id := params["id"]

		cadFile, err := c.cadFileService.Find(id)
		if err != nil {
			res := helper.BuildErrorResponse("Process failed", "CAD file not found", helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		deleteCount, err := c.processingPlanService.Delete(id)
		if err != nil {
			res := helper.BuildErrorResponse("Processing plan deletion failed", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		deleteCount, err = c.cadFileService.Delete(id)
		if err != nil {
			res := helper.BuildErrorResponse("Process failed", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		if 0 == deleteCount {
			res := helper.BuildErrorResponse("File not found: ", err.Error(), helper.EmptyObj{})
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(res)
			return
		}

		helper.DeleteFile(cadFile.StepURL)
		helper.DeleteFile(cadFile.ObjpURL)

		res := helper.BuildResponse(true, "OK!", deleteCount)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		return
	}
}
