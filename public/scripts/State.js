(function() {

    'use strict';

    var glm = require('gl-matrix');

    class State {
        constructor(spec) {
            if (!spec) {
                throw 'No state argument';
            }
            this.type = spec.type;
            // energy
            this.energy = (spec.energy !== undefined) ? spec.energy : 1.0;
            // attacking / defending / consuming
            this.target = spec.target;
            // seeking / fleeing position
            this.position = spec.position ? glm.vec3.fromValues(
                spec.position.x || spec.position[0] || 0,
                spec.position.y || spec.position[1] || 0,
                spec.position.z || spec.position[2] || 0) : glm.vec3.create();
            this.rotation = spec.rotation || 0;
        }
        interpolate(update, t) {
            var state = update.state;

            // iterpolate between current state and update based on a t value from 0 to 1
            var position = this.position;

            switch (state.type) {
                case 'alive':
                    // get distance vector
                    let diff = glm.vec3.sub(glm.vec3.create(), state.position, this.position);
                    // scale by t value
                    diff = glm.vec3.scale(diff, diff, t);
                    // get update position
                    position = glm.vec3.add(glm.vec3.create(), this.position, diff);
                    break;
            }

            return new State({
                type: this.type,
                target: this.target,
                position: position,
                rotation: this.rotation
            });
        }
        update(update) {
            if (update.target) {
                this.target = update.target;
            }
            if (update.position) {
                this.position = update.position;
            }
            if (update.rotation) {
                this.rotation = update.rotation;
            }
        }
    }

    module.exports = State;

}());
