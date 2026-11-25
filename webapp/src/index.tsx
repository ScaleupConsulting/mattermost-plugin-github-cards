import manifest from 'manifest';

import type {PluginRegistry} from 'types/mattermost-webapp';

import CardPost from './components/card';
import EventPost from './components/event';

import './components/github.css';

export default class Plugin {
    /**
     * Registers the renderers for card root posts and their thread replies.
     * @param registry - the webapp plugin registry
     */
    public async initialize(registry: PluginRegistry) {
        registry.registerPostTypeComponent('custom_ghc_card', CardPost);
        registry.registerPostTypeComponent('custom_ghc_event', EventPost);
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
