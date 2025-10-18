# Waybar Integration Guide for hyprorbit

This guide explains how to set up Waybar to display your active orbit's label and color using hyprorbit's streaming functionality.

## Prerequisites

1. **hyprorbit daemon running**: Ensure `hyprorbitd` is active
2. **Waybar installed**: This guide assumes Waybar is already configured
3. **Working hyprorbit config**: Your `~/.config/hyprorbit/config.yaml` should have orbits with labels

Note: hyprorbits can be configured to assign colors to orbits but those are not picked up by waybar with this setup. You will have to use waybars css. Hyprorbit passes an array with css classes on a per-orbit/module basis.

## Step 1: Locate Your Waybar Configuration

Waybar configuration is typically found at:
- `~/.config/waybar/config`
or
- `~/.config/waybar/config.jsonc`

## Step 2: Add the hyprorbit Module to Waybar Config

### Basic Configuration

Just add this custom module to your Waybar configuration and you are done:

```jsonc
{
  "layer": "top",
  "position": "top",

  // Add "custom/hyprorbit" to your modules list
  "modules-center": ["custom/hyprorbit#orbit", "hyprland/workspaces"],

  // Define the custom module
  "custom/hyprorbit": {
    "format": "{alt}-{text}",
    "return-type": "json",
    "exec": "hyprorbit module watch --waybar",
    "restart-interval": 0,
    "escape": true
  }
}
```

### Advanced Configuration (optional)

For more control over the display you can reference a hyprorbit waybar config. There you can control what css classes are associated with active modules or orbits and what text to send to waybar:

```yaml
module_watch:
  # default displayed text
  text: ["orbit_label"]
  tooltip: ["orbit", "workspace"]
  # alt is used for resolving "format-icons": in waybar and can be displayed via {alt} 
  alt: ["module", "workspace"]
  class:
    #classes waybar adds to your custom/hyprorbit element (multiple ones allowed)
    sources: ["orbit"]
```
And format your module like this in waybar:

```jsonc
"custom/hyprorbit#orbit": {
  "format": "{icon} {alt}/{text}",
  "format-icons": {
    "default": "ø",
    "code": " ",
    "comm": " ",
    "note": " ",
  },
  "return-type": "json",
  "exec": "hyprorbit module watch --waybar --waybar-config ~/.config/hyprorbit/waybar-orbit.yaml",
  "restart-interval": 0,
  "escape": true,
  "max-length": 20,
  "on-click": "hyprorbit orbit next",
  "on-click-right": "hyprorbit orbit prev",
  "tooltip": true
},
```

Note: hyprorbit fetches the values in the order they are declared in the arrays. It fallbacks to the next one if the first one is not available. You can choose from:
- module (name of the module (workspace))
- orbit (name of the context)
- orbit_label (alternative name for the orbit for displaying purposess)
- workspace (name of the workspace like its derived from hyprland directly)

Restart `hyprorbit module watch --waybar` (or Waybar itself) to pick up configuration changes.

## Step 3: Add CSS Styling (Optional)

Create or edit `~/.config/waybar/style.css` to style your orbit indicator:

By default your orbits are <code>alpha, beta, gamma</code> and default modules are <code>flex, code, surf, comm, note</code> You can configure the classes that hyprorbit sends to waybar. by default its the name of the orbit and the name of the module. so you can apply styles by using the orbit name and module name.

```css
#custom-hyprorbit {
	padding: 0 12px;
	margin: 0;
	border-radius: 0;
  font-weight: bold;
  color: white;
}

/* Style specific modules */
/* #custom-hyprorbits.code {
    border-bottom: 2px solid #61dafb;
} */

#custom-hyprorbit.alpha {
    border-bottom: 2px solid #CC0848;
}

#custom-hyprorbit.beta {
    border-bottom: 2px solid #e27b15;
}

#custom-hyprorbit.gamma {
    border-bottom: 2px solid #61dafb;
}

#custom-hyprorbit.delta {
    border-bottom: 2px solid #cf61fb;
}

#custom-hyprorbit.epsilon {
    border-bottom: 2px solid #6efb61;
}

#custom-hyprorbit.zeta {
    border-bottom: 2px solid #e6fb61;
}

#custom-hyprorbit.eta {
    border-bottom: 2px solid #2e40e8;
}

#custom-hyprorbit.theta {
    border-bottom: 2px solid #10f8ca;
}

#custom-hyprorbit.iota {
    border-bottom: 2px solid #ffffff;
}


#custom-hyprorbit.dev {
    border-top: 2px solid #ff6b6b;
}

/* Hover effects */
#custom-hyprorbit:hover {
    opacity: 0.7;
    /* cursor: pointer; */
}
```

## Step 4: Restart Waybar

After making changes, restart Waybar:

```bash
killall waybar
waybar &
```

Or if using systemd:

```bash
systemctl --user restart waybar
```

## Multiple Configs

You can create more than one instances of hyprorbit custom modules. that allows you to use symbols or color code modules and orbits independently in your waybar. If you want different datasets for each of these instances you can pass different configs to the each execution of hyprorbit modules like this:

```jsonc
"custom/hyprorbit#orbit": {
  "exec": "hyprorbit module watch --waybar --config ~/.config/hyprorpbits/config_orbit.yaml",
},

"custom/hyprorbit#module": {
  "exec": "hyprorbit module watch --waybar --config ~/.config/hyprorpbits/config_module.yaml",
},
```

That allows you to have hyprorbit send a different set of css classes and text attributes to waybar, depending on the module and style the elements more explicitly.
