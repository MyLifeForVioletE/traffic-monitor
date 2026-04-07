`timescale 1ns / 1ns
//////////////////////////////////////////////////////////////////////////////////
// Company: 
// Engineer: 
// 
// Create Date: 2021/10/25 18:58:45
// Design Name: 
// Module Name: udp_top_tb
// Project Name: 
// Target Devices: 
// Tool Versions: 
// Description: 
// 
// Dependencies: 
// 
// Revision:
// Revision 0.01 - File Created
// Additional Comments:
// 
//////////////////////////////////////////////////////////////////////////////////


module udp_top_tb();
reg rst_n;
reg gmii_rx_clk;
reg gmii_rx_dv;
reg [7:0]gmii_rxd;
integer dti_fid;
reg [100:0]count;
wire rec_pkt_done;
wire rec_en;
wire [31:0]rec_data;
wire [31:0]src;
wire [31:0]dst;
always #4 gmii_rx_clk = ~gmii_rx_clk;
initial begin
    rst_n = 0;
    gmii_rx_clk = 0;
    gmii_rx_dv = 0;
    count = 0;
    dti_fid = $fopen("D:/udp_read.txt","r");
    #200
    rst_n = 1;
    gmii_rx_dv <= 1'b1;
    forever begin
        @(posedge gmii_rx_clk or negedge rst_n)
        $fscanf(dti_fid,"%b %b",gmii_rxd);
        if(gmii_rxd == 8'hdd)
            count <= count+1'b1;
            if(count == 13)
                gmii_rx_dv <= 1'b0;
            if(count == 26)
                gmii_rx_dv <= 1'b1;
        if(gmii_rxd != 8'hdd)
            count <= 1'b0;
    end
        
end
udp_top u_udp_top(
    .rst_n(rst_n),
    .gmii_rx_clk(gmii_rx_clk),
    .gmii_rx_dv(gmii_rx_dv),
    .gmii_rxd(gmii_rxd),
    .rec_pkt_done(rec_pkt_done),
    .rec_en(rec_en),
    .rec_data(rec_data),
    .src(src),
    .dst(dst)
    );
endmodule
