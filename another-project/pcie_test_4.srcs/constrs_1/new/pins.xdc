#PCIe reference clock 100MHz
set_property PACKAGE_PIN F6 [get_ports {pcie_ref_clk_p[0]}]

# PCIe MGT interface
set_property LOC GTPE2_CHANNEL_X0Y7 [get_cells {system_i/xdma_0/inst/system_xdma_0_0_pcie2_to_pcie3_wrapper_i/pcie2_ip_i/inst/inst/gt_top_i/pipe_wrapper_i/pipe_lane[0].gt_wrapper_i/gtp_channel.gtpe2_channel_i}]
set_property PACKAGE_PIN D9 [get_ports {pcie_mgt_rxp[0]}]
set_property PACKAGE_PIN D7 [get_ports {pcie_mgt_txp[0]}]
set_property LOC GTPE2_CHANNEL_X0Y6 [get_cells {system_i/xdma_0/inst/system_xdma_0_0_pcie2_to_pcie3_wrapper_i/pcie2_ip_i/inst/inst/gt_top_i/pipe_wrapper_i/pipe_lane[1].gt_wrapper_i/gtp_channel.gtpe2_channel_i}]
set_property PACKAGE_PIN B10 [get_ports {pcie_mgt_rxp[1]}]
set_property PACKAGE_PIN B6 [get_ports {pcie_mgt_txp[1]}]

# PCIe rst signal
set_property -dict {PACKAGE_PIN N15 IOSTANDARD LVCMOS33} [get_ports pcie_rst_n]

# irq ack signal
set_property -dict {PACKAGE_PIN V7 IOSTANDARD LVCMOS15} [get_ports irq_ack]

#SPI 相关设置用于程序固化
set_property CFGBVS VCCO [current_design]
set_property CONFIG_VOLTAGE 3.3 [current_design]
set_property CONFIG_MODE SPIx4 [current_design]
set_property BITSTREAM.CONFIG.CONFIGRATE 50 [current_design]
set_property BITSTREAM.CONFIG.SPI_BUSWIDTH 4 [current_design]
set_property BITSTREAM.CONFIG.UNUSEDPIN PULLUP [current_design]
set_property BITSTREAM.CONFIG.SPI_FALL_EDGE YES [current_design]
set_property BITSTREAM.GENERAL.COMPRESS TRUE [current_design]

create_clock -period 20.000 -name sys_clk [get_ports sys_clk]
create_clock -period 8.000 -name eth1_rxc -waveform {0.000 4.000} [get_ports eth1_rxc]
create_clock -period 8.000 -name eth2_rxc -waveform {0.000 4.000} [get_ports eth2_rxc]
#create_clock -period 8.000 -name eth1_txc -waveform {2.000 6.000} [get_ports eth1_txc]
#create_clock -period 8.000 -name eth2_txc -waveform {2.000 6.000} [get_ports eth2_txc]

set_property -dict {PACKAGE_PIN R4 IOSTANDARD LVCMOS15} [get_ports sys_clk]
set_property -dict {PACKAGE_PIN U7 IOSTANDARD LVCMOS15} [get_ports sys_rst_n]

# 核心板引脚
set_property -dict {PACKAGE_PIN U20 IOSTANDARD LVCMOS33} [get_ports eth1_rxc]
set_property -dict {PACKAGE_PIN AA20 IOSTANDARD LVCMOS33} [get_ports eth1_rx_ctl]
set_property -dict {PACKAGE_PIN AA21 IOSTANDARD LVCMOS33} [get_ports {eth1_rxd[0]}]
set_property -dict {PACKAGE_PIN V20 IOSTANDARD LVCMOS33} [get_ports {eth1_rxd[1]}]
set_property -dict {PACKAGE_PIN U22 IOSTANDARD LVCMOS33} [get_ports {eth1_rxd[2]}]
set_property -dict {PACKAGE_PIN V22 IOSTANDARD LVCMOS33} [get_ports {eth1_rxd[3]}]

set_property -dict {PACKAGE_PIN V18 IOSTANDARD LVCMOS33} [get_ports eth1_txc]
set_property -dict {PACKAGE_PIN V19 IOSTANDARD LVCMOS33} [get_ports eth1_tx_ctl]
set_property -dict {PACKAGE_PIN T21 IOSTANDARD LVCMOS33} [get_ports {eth1_txd[0]}]
set_property -dict {PACKAGE_PIN U21 IOSTANDARD LVCMOS33} [get_ports {eth1_txd[1]}]
set_property -dict {PACKAGE_PIN P19 IOSTANDARD LVCMOS33} [get_ports {eth1_txd[2]}]
set_property -dict {PACKAGE_PIN R19 IOSTANDARD LVCMOS33} [get_ports {eth1_txd[3]}]

# 底板引脚
set_property -dict {PACKAGE_PIN N20 IOSTANDARD LVCMOS33} [get_ports eth2_rst_n]
set_property -dict {PACKAGE_PIN Y18 IOSTANDARD LVCMOS33} [get_ports eth2_rxc]
set_property -dict {PACKAGE_PIN Y21 IOSTANDARD LVCMOS33} [get_ports eth2_rx_ctl]
set_property -dict {PACKAGE_PIN Y22 IOSTANDARD LVCMOS33} [get_ports {eth2_rxd[0]}]
set_property -dict {PACKAGE_PIN AB21 IOSTANDARD LVCMOS33} [get_ports {eth2_rxd[1]}]
set_property -dict {PACKAGE_PIN AB22 IOSTANDARD LVCMOS33} [get_ports {eth2_rxd[2]}]
set_property -dict {PACKAGE_PIN Y19 IOSTANDARD LVCMOS33} [get_ports {eth2_rxd[3]}]

set_property -dict {PACKAGE_PIN P20 IOSTANDARD LVCMOS33} [get_ports eth2_txc]
set_property -dict {PACKAGE_PIN T20 IOSTANDARD LVCMOS33} [get_ports eth2_tx_ctl]
set_property -dict {PACKAGE_PIN W21 IOSTANDARD LVCMOS33} [get_ports {eth2_txd[0]}]
set_property -dict {PACKAGE_PIN W22 IOSTANDARD LVCMOS33} [get_ports {eth2_txd[1]}]
set_property -dict {PACKAGE_PIN W19 IOSTANDARD LVCMOS33} [get_ports {eth2_txd[2]}]
set_property -dict {PACKAGE_PIN W20 IOSTANDARD LVCMOS33} [get_ports {eth2_txd[3]}]

#[DRC PLIDC-3] IDELAYCTRLs in same group have conflicting connections: IDELAYCTRL cells 'u1_gmii_to_rgmii/u_rgmii_rx1/IDELAYCTRL_inst1' and 'u2_gmii_to_rgmii/u_rgmii_rx2/IDELAYCTRL_inst2' have same IODELAY_GROUP 'rgmii_rx_delay1' but their RST signals are different

set_property IODELAY_GROUP rgmii_rx_delay1 [get_cells u1_gmii_to_rgmii/u_rgmii_rx1/IDELAYCTRL_inst1/*delay*]
set_property IODELAY_GROUP rgmii_rx_delay1 [get_cells u2_gmii_to_rgmii/u_rgmii_rx2/IDELAYCTRL_inst2/*delay*]

