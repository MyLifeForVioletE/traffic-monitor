`timescale 1 ps / 1 ps

module design_1_wrapper
   (
     //核心板以太网RGMII接口   
    input              eth1_rxc   , //RGMII接收数据时钟
    input              eth1_rx_ctl, //RGMII输入数据有效信号
    input       [3:0]  eth1_rxd   , //RGMII输入数据
    
    output             eth1_txc   , //RGMII发送数据时钟    
    output             eth1_tx_ctl, //RGMII输出数据有效信号
    output      [3:0]  eth1_txd   , //RGMII输出数据
               
    
    //底板以太网RGMII接口   
    input              eth2_rxc   , //RGMII接收数据时钟
    input              eth2_rx_ctl, //RGMII输入数据有效信号
    input       [3:0]  eth2_rxd   , //RGMII输入数据
    
    output             eth2_txc   , //RGMII发送数据时钟    
    output             eth2_tx_ctl, //RGMII输出数据有效信号
    output      [3:0]  eth2_txd   , //RGMII输出数据 
    
    output             eth2_rst_n , //以太网芯片复位信号，低电平有效 
    
    input              sys_clk   , //系统时钟
    input              sys_rst_n , //系统复位信号，低电平有效 
	input 	[0:0]	pcie_ref_clk_n,
	input 	[0:0]	pcie_ref_clk_p,
	input 			pcie_rst_n,
	
	input  	[1:0]	pcie_mgt_rxn,
	input  	[1:0]	pcie_mgt_rxp,
	output 	[1:0]	pcie_mgt_txn,
	output 	[1:0]	pcie_mgt_txp
  
  );
  parameter IDELAY_VALUE = 0;
  wire [63:0]M_AXIS_0_tdata;
  wire M_AXIS_0_tready;
  wire M_AXIS_0_tvalid;
  wire [63:0]S_AXIS_0_tdata;
  wire S_AXIS_0_tready;
  wire S_AXIS_0_tvalid;

wire          clk_200m   ; //用于IO延时的时钟 

wire gmii_rx_clk1;
wire gmii_rx_dv1;
wire  [7:0]   gmii_rxd1;
wire gmii_tx_clk1;
wire gmii_tx_en1;
wire  [7:0]   gmii_txd1;

wire gmii_rx_clk2;
wire gmii_rx_dv2;
wire  [7:0]   gmii_rxd2;
wire gmii_tx_clk2;
wire gmii_tx_en2;
wire  [7:0]   gmii_txd2;

wire [31:0] src;
wire [31:0] dst;
wire rec_pkt_done;
reg send_en;
wire [63:0] dout;

  design_1 design_1_i
       (.M_AXIS_0_tdata(), //receive
        .M_AXIS_0_tready(),
        .M_AXIS_0_tvalid(),
        .S_AXIS_0_tdata(dout),  //send
        .S_AXIS_0_tready(),
        .S_AXIS_0_tvalid(send_en),
        .led_tri_o(),
        .lnk_up_led(),
        .pcie_mgt_rxn(pcie_mgt_rxn),
        .pcie_mgt_rxp(pcie_mgt_rxp),
        .pcie_mgt_txn(pcie_mgt_txn),
        .pcie_mgt_txp(pcie_mgt_txp),
        .pcie_ref_clk_n(pcie_ref_clk_n),
        .pcie_ref_clk_p(pcie_ref_clk_p),
        .pcie_rst_n(pcie_rst_n));



wire clk_125;

clk_wiz_0 u_clk0
(
// Clock out ports
.clk_out1(clk_125),     // output clk_out1
// Clock in ports
.clk_in1(sys_clk));      // input clk_in1

clk_wiz_1 u_clk1
(
// Clock out ports
.clk_out1(clk_200m),     // output clk_out1
// Clock in ports
.clk_in1(sys_clk));      // input clk_in1


gmii_2_rgmii 
    #(
     .IDELAY_VALUE (IDELAY_VALUE)
     )
    u1_gmii_to_rgmii(
    .sys_clk        (sys_clk),
    .idelay_clk    (clk_200m    ),
    .rst_n (sys_rst_n),

    .gmii_rx_clk   (gmii_rx_clk1 ),//output
    .gmii_rx_dv    (gmii_rx_dv1  ),//output
    .gmii_rxd      (gmii_rxd1    ),//output
    .gmii_tx_clk   (gmii_tx_clk1 ),//output
    
    .gmii_tx_en    (gmii_tx_en1  ),//input
    .gmii_txd      (gmii_txd1    ),//input
    
    .rgmii_rxc     (eth1_rxc     ),//input
    .rgmii_rx_ctl  (eth1_rx_ctl  ),//input
    .rgmii_rxd     (eth1_rxd     ),//input
    
    .rgmii_txc     (eth2_txc     ),//output
    .rgmii_tx_ctl  (eth2_tx_ctl  ),//output
    .rgmii_txd     (eth2_txd     ),//output
    
    .rec_pkt_done  (rec_pkt_done ),
    .src           (src          ),
    .dst           (dst          )
    );

gmii_to_rgmii 
    #(
     .IDELAY_VALUE (IDELAY_VALUE)
     )
    u2_gmii_to_rgmii(
    .idelay_clk    (clk_200m    ),
    //output
    .gmii_rx_clk   (gmii_rx_clk2 ),
    .gmii_rx_dv    (gmii_rx_dv2  ),
    .gmii_rxd      (gmii_rxd2    ),
    .gmii_tx_clk   (gmii_tx_clk2 ),
    //input
    .gmii_tx_en    (gmii_tx_en2  ),
    .gmii_txd      (gmii_txd2    ),
    //input
    .rgmii_rxc     (eth2_rxc     ),
    .rgmii_rx_ctl  (eth2_rx_ctl  ),
    .rgmii_rxd     (eth2_rxd     ),
    //output
    .rgmii_txc     (eth1_txc     ),
    .rgmii_tx_ctl  (eth1_tx_ctl  ),
    .rgmii_txd     (eth1_txd     )
    );

assign gmii_tx_en1 = gmii_rx_dv1;
assign gmii_txd1 =  gmii_rxd1;

assign gmii_tx_en2 = gmii_rx_dv2;
assign gmii_txd2 =  gmii_rxd2;

wire full;
wire empty;


fifo_generator_0 u_fifo (
  .wr_clk(gmii_rx_clk1),  // input wire wr_clk
  .rd_clk(clk_125),  // input wire rd_clk
  .din({src,dst}),        // input wire [63 : 0] din
  .wr_en(rec_pkt_done),    // input wire wr_en
  .rd_en(send_en),    // input wire rd_en
  .dout(dout),      // output wire [63 : 0] dout
  .full(full),      // output wire full
  .empty(empty)    // output wire empty
);

always@(posedge clk_125)
begin
	if(!pcie_rst_n)
	begin
		  
		send_en 	<= 1'b0;
	end
	else
	   if (!empty) begin
	       send_en 	<= 1'b1;
	   end
	   else begin
	       send_en 	<= 1'b0;
	   end
end


endmodule
