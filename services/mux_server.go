package services

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"proxy/utils"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type MuxServer struct {
	cfg    MuxServerArgs
	udpChn chan MuxUDPItem
	sc     utils.ServerChannel
}

type MuxServerManager struct {
	cfg      MuxServerArgs
	udpChn   chan MuxUDPItem
	sc       utils.ServerChannel
	serverID string
	// cm       utils.ConnManager
}

func NewMuxServerManager() Service {
	return &MuxServerManager{
		cfg:      MuxServerArgs{},
		udpChn:   make(chan MuxUDPItem, 50000),
		serverID: utils.Uniqueid(),
		// cm:       utils.NewConnManager(),
	}
}
func (s *MuxServerManager) Start(args interface{}) (err error) {
	s.cfg = args.(MuxServerArgs)
	s.CheckArgs()
	if *s.cfg.Parent != "" {
		log.Printf("use tls parent %s", *s.cfg.Parent)
	} else {
		log.Fatalf("parent required")
	}

	s.InitService()

	log.Printf("server id: %s", s.serverID)
	//log.Printf("route:%v", *s.cfg.Route)
	for _, _info := range *s.cfg.Route {
		IsUDP := *s.cfg.IsUDP
		if strings.HasPrefix(_info, "udp://") {
			IsUDP = true
		}
		info := strings.TrimPrefix(_info, "udp://")
		info = strings.TrimPrefix(info, "tcp://")
		_routeInfo := strings.Split(info, "@")
		server := NewMuxServer()
		local := _routeInfo[0]
		remote := _routeInfo[1]
		KEY := *s.cfg.Key
		if strings.HasPrefix(remote, "[") {
			KEY = remote[1:strings.LastIndex(remote, "]")]
			remote = remote[strings.LastIndex(remote, "]")+1:]
		}
		if strings.HasPrefix(remote, ":") {
			remote = fmt.Sprintf("127.0.0.1%s", remote)
		}
		err = server.Start(MuxServerArgs{
			CertBytes: s.cfg.CertBytes,
			KeyBytes:  s.cfg.KeyBytes,
			Parent:    s.cfg.Parent,
			CertFile:  s.cfg.CertFile,
			KeyFile:   s.cfg.KeyFile,
			Local:     &local,
			IsUDP:     &IsUDP,
			Remote:    &remote,
			Key:       &KEY,
			Timeout:   s.cfg.Timeout,
			Mgr:       s,
		})

		if err != nil {
			return
		}
	}
	return
}
func (s *MuxServerManager) Clean() {
	s.StopService()
}
func (s *MuxServerManager) StopService() {
	// s.cm.RemoveAll()
}
func (s *MuxServerManager) CheckArgs() {
	if *s.cfg.CertFile == "" || *s.cfg.KeyFile == "" {
		log.Fatalf("cert and key file required")
	}
	s.cfg.CertBytes, s.cfg.KeyBytes = utils.TlsBytes(*s.cfg.CertFile, *s.cfg.KeyFile)
}
func (s *MuxServerManager) InitService() {
	// s.InitHeartbeatDeamon()
}

// func (s *MuxServerManager) InitHeartbeatDeamon() {
// 	log.Printf("heartbeat started")
// 	go func() {
// 		var heartbeatConn net.Conn
// 		var ID string
// 		for {
// 			//close all connection
// 			s.cm.Remove(ID)
// 			utils.CloseConn(&heartbeatConn)
// 			heartbeatConn, ID, err := s.GetOutConn(CONN_SERVER_HEARBEAT)
// 			if err != nil {
// 				log.Printf("heartbeat connection err: %s, retrying...", err)
// 				time.Sleep(time.Second * 3)
// 				utils.CloseConn(&heartbeatConn)
// 				continue
// 			}
// 			log.Printf("heartbeat connection created,id:%s", ID)
// 			writeDie := make(chan bool)
// 			readDie := make(chan bool)
// 			go func() {
// 				for {
// 					heartbeatConn.SetWriteDeadline(time.Now().Add(time.Second * 3))
// 					_, err = heartbeatConn.Write([]byte{0x00})
// 					heartbeatConn.SetWriteDeadline(time.Time{})
// 					if err != nil {
// 						log.Printf("heartbeat connection write err %s", err)
// 						break
// 					}
// 					time.Sleep(time.Second * 3)
// 				}
// 				close(writeDie)
// 			}()
// 			go func() {
// 				for {
// 					signal := make([]byte, 1)
// 					heartbeatConn.SetReadDeadline(time.Now().Add(time.Second * 6))
// 					_, err := heartbeatConn.Read(signal)
// 					heartbeatConn.SetReadDeadline(time.Time{})
// 					if err != nil {
// 						log.Printf("heartbeat connection read err: %s", err)
// 						break
// 					} else {
// 						// log.Printf("heartbeat from bridge")
// 					}
// 				}
// 				close(readDie)
// 			}()
// 			select {
// 			case <-readDie:
// 			case <-writeDie:
// 			}
// 		}
// 	}()
// }
func (s *MuxServerManager) GetOutConn(typ uint8) (outConn net.Conn, ID string, err error) {
	outConn, err = s.GetConn()
	if err != nil {
		log.Printf("connection err: %s", err)
		return
	}
	ID = s.serverID
	_, err = outConn.Write(utils.BuildPacket(typ, s.serverID))
	if err != nil {
		log.Printf("write connection data err: %s ,retrying...", err)
		utils.CloseConn(&outConn)
		return
	}
	return
}
func (s *MuxServerManager) GetConn() (conn net.Conn, err error) {
	var _conn tls.Conn
	_conn, err = utils.TlsConnectHost(*s.cfg.Parent, *s.cfg.Timeout, s.cfg.CertBytes, s.cfg.KeyBytes)
	if err == nil {
		conn = net.Conn(&_conn)
	}
	return
}
func NewMuxServer() Service {
	return &MuxServer{
		cfg:    MuxServerArgs{},
		udpChn: make(chan MuxUDPItem, 50000),
	}
}

type MuxUDPItem struct {
	packet    *[]byte
	localAddr *net.UDPAddr
	srcAddr   *net.UDPAddr
}

func (s *MuxServer) InitService() {
	s.UDPConnDeamon()
}
func (s *MuxServer) CheckArgs() {
	if *s.cfg.Remote == "" {
		log.Fatalf("remote required")
	}
}

func (s *MuxServer) Start(args interface{}) (err error) {
	s.cfg = args.(MuxServerArgs)
	s.CheckArgs()
	s.InitService()
	host, port, _ := net.SplitHostPort(*s.cfg.Local)
	p, _ := strconv.Atoi(port)
	s.sc = utils.NewServerChannel(host, p)
	if *s.cfg.IsUDP {
		err = s.sc.ListenUDP(func(packet []byte, localAddr, srcAddr *net.UDPAddr) {
			s.udpChn <- MuxUDPItem{
				packet:    &packet,
				localAddr: localAddr,
				srcAddr:   srcAddr,
			}
		})
		if err != nil {
			return
		}
		log.Printf("proxy on udp tunnel server mode %s", (*s.sc.UDPListener).LocalAddr())
	} else {
		err = s.sc.ListenTCP(func(inConn net.Conn) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("tserver conn handler crashed with err : %s \nstack: %s", err, string(debug.Stack()))
				}
			}()
			var outConn net.Conn
			var ID string
			for {
				outConn, ID, err = s.GetOutConn(CONN_SERVER)
				if err != nil {
					utils.CloseConn(&outConn)
					log.Printf("connect to %s fail, err: %s, retrying...", *s.cfg.Parent, err)
					time.Sleep(time.Second * 3)
					continue
				} else {
					break
				}
			}
			utils.IoBind(inConn, outConn, func(err interface{}) {
				// s.cfg.Mgr.cm.RemoveOne(s.cfg.Mgr.serverID, ID)
				log.Printf("%s conn %s released", *s.cfg.Key, ID)
			})
			//add conn
			// s.cfg.Mgr.cm.Add(s.cfg.Mgr.serverID, ID, &inConn)
			log.Printf("%s conn %s created", *s.cfg.Key, ID)
		})
		if err != nil {
			return
		}
		log.Printf("proxy on tunnel server mode %s", (*s.sc.Listener).Addr())
	}
	return
}
func (s *MuxServer) Clean() {

}
func (s *MuxServer) GetOutConn(typ uint8) (outConn net.Conn, ID string, err error) {
	outConn, err = s.GetConn()
	if err != nil {
		log.Printf("connection err: %s", err)
		return
	}
	remoteAddr := "tcp:" + *s.cfg.Remote
	if *s.cfg.IsUDP {
		remoteAddr = "udp:" + *s.cfg.Remote
	}
	ID = utils.Uniqueid()
	_, err = outConn.Write(utils.BuildPacket(typ, *s.cfg.Key, ID, remoteAddr, s.cfg.Mgr.serverID))
	if err != nil {
		log.Printf("write connection data err: %s ,retrying...", err)
		utils.CloseConn(&outConn)
		return
	}
	return
}
func (s *MuxServer) GetConn() (conn net.Conn, err error) {
	var _conn tls.Conn
	_conn, err = utils.TlsConnectHost(*s.cfg.Parent, *s.cfg.Timeout, s.cfg.CertBytes, s.cfg.KeyBytes)
	if err == nil {
		conn = net.Conn(&_conn)
	}
	return
}
func (s *MuxServer) UDPConnDeamon() {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("udp conn deamon crashed with err : %s \nstack: %s", err, string(debug.Stack()))
			}
		}()
		var outConn net.Conn
		// var hb utils.HeartbeatReadWriter
		var ID string
		// var cmdChn = make(chan bool, 1000)
		var err error
		for {
			item := <-s.udpChn
		RETRY:
			if outConn == nil {
				for {
					outConn, ID, err = s.GetOutConn(CONN_SERVER)
					if err != nil {
						// cmdChn <- true
						outConn = nil
						utils.CloseConn(&outConn)
						log.Printf("connect to %s fail, err: %s, retrying...", *s.cfg.Parent, err)
						time.Sleep(time.Second * 3)
						continue
					} else {
						go func(outConn net.Conn, ID string) {
							go func() {
								// <-cmdChn
								// outConn.Close()
							}()
							for {
								srcAddrFromConn, body, err := utils.ReadUDPPacket(outConn)
								if err == io.EOF || err == io.ErrUnexpectedEOF {
									log.Printf("UDP deamon connection %s exited", ID)
									break
								}
								if err != nil {
									log.Printf("parse revecived udp packet fail, err: %s ,%v", err, body)
									continue
								}
								//log.Printf("udp packet revecived over parent , local:%s", srcAddrFromConn)
								_srcAddr := strings.Split(srcAddrFromConn, ":")
								if len(_srcAddr) != 2 {
									log.Printf("parse revecived udp packet fail, addr error : %s", srcAddrFromConn)
									continue
								}
								port, _ := strconv.Atoi(_srcAddr[1])
								dstAddr := &net.UDPAddr{IP: net.ParseIP(_srcAddr[0]), Port: port}
								_, err = s.sc.UDPListener.WriteToUDP(body, dstAddr)
								if err != nil {
									log.Printf("udp response to local %s fail,ERR:%s", srcAddrFromConn, err)
									continue
								}
								//log.Printf("udp response to local %s success , %v", srcAddrFromConn, body)
							}
						}(outConn, ID)
						break
					}
				}
			}
			outConn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = outConn.Write(utils.UDPPacket(item.srcAddr.String(), *item.packet))
			outConn.SetWriteDeadline(time.Time{})
			if err != nil {
				utils.CloseConn(&outConn)
				outConn = nil
				log.Printf("write udp packet to %s fail ,flush err:%s ,retrying...", *s.cfg.Parent, err)
				goto RETRY
			}
			//log.Printf("write packet %v", *item.packet)
		}
	}()
}
