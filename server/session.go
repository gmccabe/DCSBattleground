package server

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
	//"fmt"
	"github.com/lib/pq"
	//"strconv"
)

type sessionRadarSnapshotData struct {
	Offset  int64          `json:"offset"`
	Created []*StateObject `json:"created"`
	Updated []*StateObject `json:"updated"`
	Deleted []uint64       `json:"deleted"`
}

type sessionStateData struct {
	SessionId string         `json:"session_id"`
	Offset    int64          `json:"offset"`
	Objects   []*StateObject `json:"objects"`
}

type serverSession struct {
	sync.Mutex

	server *TacViewServerConfig
	

	subscriberIdx int
	subscribers   map[int]chan<- []byte
	state         sessionState
}

type sharedGeometry struct {
	Add		[]geometry
	Delete	[]int
	Recon	[]geometry
}

type player struct {
	DiscordId string
	PlayerName string
}

type players struct {
	Inflight []player
}

func newServerSession(server *TacViewServerConfig) (*serverSession, error) {
	return &serverSession{server: server, subscribers: make(map[int]chan<- []byte)}, nil
}

type PlayerMetadata struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s *serverSession) GetPlayerList() []PlayerMetadata {
	players := []PlayerMetadata{}
	s.state.RLock()
	for _, object := range s.state.objects {
		isPlayer := false

		for _, typeName := range object.Types {
			if typeName == "Air" {
				isPlayer = true
				continue
			}
		}
		if !isPlayer {
			continue
		}

		pilotName, ok := object.Properties["Pilot"]
		if !ok {
			continue
		}

		if strings.HasPrefix(pilotName, object.Properties["Group"]) {
			continue
		}

		players = append(players, PlayerMetadata{
			Name: pilotName,
			Type: object.Properties["Name"],
		})
	}
	s.state.RUnlock()
	return players
}

func (s *serverSession) updateLoop() {
	refreshRate := time.Duration(5)
	if s.server.RadarRefreshRate != 0 {
		refreshRate = time.Duration(s.server.RadarRefreshRate)
	}
	ticker := time.NewTicker(time.Second * refreshRate)

	var currentOffset int64
	for {
		<-ticker.C

		if !s.state.active {
			continue
		}

		s.state.Lock()
		data := &sessionRadarSnapshotData{
			Offset:  s.state.offset,
			Created: make([]*StateObject, 0),
			Updated: make([]*StateObject, 0),
			Deleted: make([]uint64, 0),
		}

		for _, object := range s.state.objects {
			if object.Deleted {
				data.Deleted = append(data.Deleted, object.Id)
			} else if object.CreatedAt > currentOffset {
				data.Created = append(data.Created, object)
			} else if object.UpdatedAt > currentOffset {
				data.Updated = append(data.Updated, object)
			}
		}

		// We can now delete these objects from the state
		for _, objectId := range data.Deleted {
			delete(s.state.objects, objectId)
		}

		currentOffset = s.state.offset
		s.state.Unlock()

		s.publish("SESSION_RADAR_SNAPSHOT", data)
		
		s.runSharedGeometry()
		s.runConnectedPlayer()
	}
}

func (s *serverSession) getInitialState() (*sessionStateData, []*StateObject) {
	s.state.RLock()
	defer s.state.RUnlock()

	if !s.state.active {
		return nil, nil
	}

	objects := make([]*StateObject, len(s.state.objects))

	idx := 0
	for _, object := range s.state.objects {
		objects[idx] = object
		idx += 1
	}

	return &sessionStateData{
		SessionId: s.state.sessionId,
		Offset:    s.state.offset,
	}, objects
}

func (s *serverSession) publish(event string, data interface{}) error {
	encoded, err := json.Marshal(map[string]interface{}{
		"e": event,
		"d": data,
	})
	if err != nil {
		return err
	}

	s.Lock()
	for id, sub := range s.subscribers {
		select {
		case sub <- encoded:
			continue
		default:
			log.Printf("[session:%v] subscriber %v non-responsive, closing", s.server.Name, id)
			delete(s.subscribers, id)
			close(sub)
		}
	}
	s.Unlock()
	return nil

}

func (s *serverSession) run() {
	go s.updateLoop()

	for {
		s.runTacViewClient()
		//s.runSharedGeometry()
		time.Sleep(time.Second * 5)
	}
}

func (s *serverSession) runConnectedPlayer() error {
	var DcsName = s.server.DcsName
	//var ViewAircraftWhenInFlight = s.server.ViewAircraftWhenInFlight
	
	Player := player{}
	Player.DiscordId="NADA"
	Player.PlayerName="NADA"
	Players := []player{}
	Players = append(Players, Player)
	
	err := db.Ping()
	if err == nil {
		rows, err2 := db.Query(`SELECT discord_id, players.name FROM statistics, players, missions WHERE players.ucid = statistics.player_ucid AND statistics.mission_id = missions.id AND hop_off is null AND server_name = '` + DcsName + `'`)
		//fmt.Println(`SELECT discord_id, players.name FROM statistics, players, missions WHERE players.ucid = statistics.player_ucid AND statistics.mission_id = missions.id AND hop_off is null AND server_name = '` + DcsName + `'`)
		CheckError(err2)
		defer rows.Close()
		for rows.Next() {
			var discordId string
			var playerName string
			
			err = rows.Scan(&discordId, &playerName)
			CheckError(err)

			Player.DiscordId = discordId
			Player.PlayerName = playerName
			Players = append(Players, Player)
		}
	}
	PlayersF := players{Inflight: Players}
	s.publish("SESSION_PLAYERS_IN_SLOT", PlayersF)
	return nil
}

func (s *serverSession) runSharedGeometry() error {
	var DcsName = s.server.DcsName
	var reconGeometry = []geometry{}
	
	err := db.Ping()
	if err == nil {
		var Id int
		var Type string
		var Name string
		var DiscordName string
		var Avatar string
		var PosMGRS string
		var Screenshot []string
		var Side string
		var Server string
		var Position []float32
		var Center []float32
		var Radius float32
		
		rows, err := db.Query(`SELECT id, type, name, discordname, avatar, posmgrs, screenshot, side, server FROM bg_geometry WHERE server='` + DcsName + `' AND type='recon'`)
		CheckError(err)
		defer rows.Close()
		for rows.Next() {
			err = rows.Scan(&Id, &Type, &Name, &DiscordName, &Avatar, &PosMGRS, pq.Array(&Screenshot), &Side, &Server)
			CheckError(err)
		
			var geo geometry
			geo.Id = Id
			geo.Type = Type
			geo.Name = Name
			geo.DiscordName = DiscordName
			geo.Avatar = Avatar
			geo.PosMGRS = PosMGRS
			geo.Screenshot = Screenshot
			geo.Side = Side
			geo.Server = Server
			reconGeometry = append(reconGeometry, geo)
		}
		
		
		
		geoListGlob = []geometry{}
		rows, err = db.Query(`SELECT id, type, name, discordname, avatar, side, server, Position, array_to_json(CASE WHEN points IS NULL THEN '{}'::numeric[] else points::numeric[] END), center, radius FROM bg_geometry WHERE server='` + DcsName + `' AND type!='recon'`)
		CheckError(err)
		defer rows.Close()
		for rows.Next() {
			var pointsStr string
			var Points [][]float32
			
			err = rows.Scan(&Id, &Type, &Name, &DiscordName, &Avatar, &Side, &Server, pq.Array(&Position), &pointsStr, pq.Array(&Center), &Radius)
			CheckError(err)
			
			
			json.Unmarshal([]byte(pointsStr), &Points)
			var geo geometry
			geo.Id = Id
			geo.Type = Type
			geo.Name = Name
			geo.DiscordName = DiscordName
			geo.Avatar = Avatar
			geo.Side = Side
			geo.Server = Server
			geo.Position = Position
			geo.Points = Points
			geo.Center = Center
			geo.Radius = Radius
			geoListGlob = append(geoListGlob, geo)
		}
	}


	sharedGeometry := sharedGeometry{Add:geoListGlob, Delete:geoListDel, Recon:reconGeometry}
	s.publish("SESSION_SHARED_GEOMETRY", sharedGeometry)
	return nil
}



func (s *serverSession) runTacViewClient() error {
	client := NewTacViewClient(s.server.Hostname, s.server.Port, s.server.Password)
	header, timeFrameStream, err := client.Start()
	if err != nil {
		s.server.Enabled = false
		//log.Printf("test2 : " + strconv.FormatBool(s.server.Enabled))
		return err
	}

	err = s.state.initialize(header)
	if err != nil {
		s.server.Enabled = false
		return err
	}

	s.server.Enabled = true

	s.state.Lock()
	objects := make([]*StateObject, len(s.state.objects))
	var idx = 0
	for _, object := range s.state.objects {
		objects[idx] = object
		idx += 1
	}
	s.state.Unlock()

	log.Printf("[session:%v] tacview client session initialized", s.server.Name)
	s.publish("SESSION_STATE", &sessionStateData{
		SessionId: s.state.sessionId,
		Objects:   objects,
	})

	for {
		timeFrame, ok := <-timeFrameStream
		if !ok {
			return nil
		}

		s.state.Lock()
		s.state.update(timeFrame)
		s.state.Unlock()
	}
}

func (s *serverSession) removeSub(id int) {
	s.Lock()
	defer s.Unlock()
	delete(s.subscribers, id)
}

func (s *serverSession) addSub() (<-chan []byte, func()) {
	sub := make(chan []byte, 16)
	s.Lock()
	id := s.subscriberIdx
	s.subscribers[id] = sub
	s.subscriberIdx += 1
	s.Unlock()
	return sub, func() {
		s.removeSub(id)
	}
}
