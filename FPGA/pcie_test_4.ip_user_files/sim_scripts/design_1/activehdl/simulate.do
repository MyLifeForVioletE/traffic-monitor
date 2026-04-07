onbreak {quit -force}
onerror {quit -force}

asim -t 1ps +access +r +m+design_1 -L xpm -L xil_defaultlib -L blk_mem_gen_v8_4_4 -L fifo_generator_v13_1_4 -L xdma_v4_1_4 -L axi_lite_ipif_v3_0_4 -L lib_cdc_v1_0_2 -L interrupt_control_v3_1_4 -L axi_gpio_v2_0_22 -L axis_infrastructure_v1_1_0 -L axis_data_fifo_v2_0_2 -L unisims_ver -L unimacro_ver -L secureip -O5 xil_defaultlib.design_1 xil_defaultlib.glbl

do {wave.do}

view wave
view structure

do {design_1.udo}

run -all

endsim

quit -force
