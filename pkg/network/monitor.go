package network

import (
	"fmt"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type Monitor struct {
	handle  *pcap.Handle
	ipStats *IPStats
	stopCh  chan struct{}
}

func NewMonitor() *Monitor {
	return &Monitor{
		stopCh: make(chan struct{}),
	}
}

func (m *Monitor) Start(deviceName string) error {
	var err error
	m.handle, err = pcap.OpenLive(deviceName, 1600, false, pcap.BlockForever)
	if err != nil {
		return fmt.Errorf("error opening device %s: %v", deviceName, err)
	}

	m.ipStats = NewIPStats(5 * time.Minute)
	go m.processPackets()
	go m.startCleanup()

	return nil
}

func (m *Monitor) Stop() {
	if m.handle != nil {
		close(m.stopCh)
		m.handle.Close()
		m.handle = nil
	}
}

func (m *Monitor) GetStats() map[string]NetworkStats {
	if m.ipStats == nil {
		return nil
	}
	return m.ipStats.GetStats()
}

func (m *Monitor) processPackets() {
	packetSource := gopacket.NewPacketSource(m.handle, m.handle.LinkType())
	for {
		select {
		case <-m.stopCh:
			return
		case packet := <-packetSource.Packets():
			m.processPacket(packet)
		}
	}
}

func (m *Monitor) processPacket(packet gopacket.Packet) {
	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return
	}

	ip, _ := ipLayer.(*layers.IPv4)
	length := uint64(len(packet.Data()))

	srcIP := ip.SrcIP.String()
	dstIP := ip.DstIP.String()

	m.ipStats.updateStats(srcIP, dstIP, length)
}

func (m *Monitor) startCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.ipStats.cleanup()
		case <-m.stopCh:
			return
		}
	}
}
