-- Force English node.description for Scarlett 2i2, regardless of system locale.
-- Without this, PipeWire uses a locale-dependent name (e.g. "Stéréo analogique"
-- in French), which breaks qpwgraph patchbay matching.
table.insert(alsa_monitor.rules, {
  matches = {
    {
      { "node.name", "matches", "alsa_*.usb-Focusrite_Scarlett_2i2_USB*" },
    },
  },
  apply_properties = {
    ["node.description"] = "Scarlett 2i2 3rd Gen Analog Stereo",
  },
})
