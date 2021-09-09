package listener

import (
	"fmt"
	"log"
	"time"

	"github.com/WilfredDube/fxtract-backend/entity"
	"github.com/WilfredDube/fxtract-backend/lib/contracts"
	"github.com/WilfredDube/fxtract-backend/lib/msgqueue"
	"github.com/WilfredDube/fxtract-backend/service"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventProcessor struct {
	EventListener         msgqueue.EventListener
	CadFileService        service.CadFileService
	TaskService           service.TaskService
	ToolService           service.ToolService
	ProcessingPlanService service.ProcessingPlanService
}

func (p *EventProcessor) ProcessEvents(events ...string) {
	log.Println("listening for events")

	received, errors, err := p.EventListener.Listen(events...)

	if err != nil {
		panic(err)
	}

	for {
		select {
		case evt := <-received:
			fmt.Printf("got event %T: \n", evt)
			p.handleEvent(evt)
		case err = <-errors:
			fmt.Printf("got error while receiving event: %s\n", err)
		}
	}
}

func (p *EventProcessor) handleEvent(event msgqueue.Event) {
	switch e := event.(type) {
	case *contracts.FeatureRecognitionComplete:
		log.Printf("event %s created: %s", e.CADFileID, e.TaskID)

		cadFile, err := p.CadFileService.Find(e.CADFileID)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to unmarshal data: ", err)
		}

		cadFile.FeatureProps = e.FeatureProps
		cadFile.BendFeatures = []entity.BendFeature{}
		cadFile.BendFeatures = e.BendFeatures

		for i, bend := range cadFile.BendFeatures {
			tool, err := p.ToolService.FindByAngle(int64(bend.Angle))
			if err != nil {
				log.Fatalf("%s: %s", "Failed to retrieve tool data: ", err)
			}

			cadFile.BendFeatures[i].ToolID = tool.ToolID
		}

		_, err = p.CadFileService.Update(*cadFile)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to update data: ", err)
		}

		task, err := p.TaskService.Find(e.TaskID)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to retrieve task data: ", err)
		}

		task.Status = entity.Complete
		task.ProcessingTime = e.FeatureProps.FRETime

		returedTask, err := p.TaskService.Update(*task)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to update data: ", err)
		}

		log.Printf("[ User: %s > TaskID: %s > Task status: %s]: CAD file (%s) features saved successfully!", e.UserID, returedTask.TaskID, returedTask.Status, e.CADFileID)
		log.Printf("==========================================================")
	case *contracts.ProcessPlanningComplete:
		log.Printf("event %s created: %s", e.CADFileID, e.TaskID)
		log.Printf("==========================================================")
		fmt.Printf("Received a Processing plan for CAD file ID: %v\n", e.ProcessingPlan.CADFileID)

		processingPlan := entity.ProcessingPlan{}
		processingPlan.ID = primitive.NewObjectID()
		processingPlan.CADFileID = e.ProcessingPlan.CADFileID

		cadFile, err := p.CadFileService.Find(processingPlan.CADFileID.Hex())
		if err != nil {
			log.Fatalf("%s: %s", "Cadfile does not exist ", err)
		}

		processingPlan.Rotations = e.ProcessingPlan.Rotations
		processingPlan.Flips = e.ProcessingPlan.Flips
		processingPlan.Tools = e.ProcessingPlan.Tools
		processingPlan.Modules = e.ProcessingPlan.Modules
		processingPlan.ProcessingTime = e.ProcessingPlan.ProcessingTime
		processingPlan.EstimatedManufacturingTime = e.ProcessingPlan.EstimatedManufacturingTime
		processingPlan.TotalToolDistance = e.ProcessingPlan.TotalToolDistance
		processingPlan.BendingSequences = e.ProcessingPlan.BendingSequences
		processingPlan.Quantity = e.ProcessingPlan.Quantity
		processingPlan.CreatedAt = time.Now().Unix()

		_, err = p.ProcessingPlanService.Create(&processingPlan)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to save processing plan: ", err)
		}

		cadFile.FeatureProps.ProcessLevel = e.ProcessLevel
		_, err = p.CadFileService.Update(*cadFile)
		if err != nil {
			log.Fatalf("%s: %s", "Cadfile update failed ", err)
		}

		task, err := p.TaskService.Find(e.TaskID)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to retrieve task data: ", err)
		}

		task.Status = entity.Complete
		task.ProcessingTime = e.ProcessingPlan.EstimatedManufacturingTime

		returedTask, err := p.TaskService.Update(*task)
		if err != nil {
			log.Fatalf("%s: %s", "Failed to update data: ", err)
		}

		log.Printf("[ User: %s > TaskID: %s > Task status: %s]: CAD file (%s) processing plan saved successfully!", e.UserID, returedTask.TaskID, returedTask.Status, e.CADFileID)
		log.Printf("==========================================================")
	default:
		log.Printf("unknown event type: %T", e)
	}
}
