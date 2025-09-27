hypr-orbits orbit get
hypr-orbits orbit next|prev
hypr-orbits orbit set <label>        # e.g. α

hypr-orbits role jump  <role>        # workspace name:<role>-<orbit>
hypr-orbits role focus <role> --match <class:^Foo$|title:^Bar$|appid:^baz$> --cmd "<spawnline>"
hypr-orbits role seed  <role>        # from config, if target workspace empty

Behavior of role focus:
1. Look for a matching window in the role’s workspace → focus it.
2. Else look anywhere → move it into role’s workspace and focus.
3. Else spawn --cmd in the role’s workspace and focus.
(If you prefer not moving existing windows, that’s a one-line policy toggle in code.)
